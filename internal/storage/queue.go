package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Queue interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
	Consume(ctx context.Context, queueName string, handler func([]byte) error) error
}

type RabbitMQQueue struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	pubCh   *amqp.Channel
	returns chan amqp.Return
	pubMu   sync.Mutex
}

func NewRabbitMQQueue(config *models.Config) (*RabbitMQQueue, error) {
	conn, err := amqp.Dial(config.RabbitMQ.URL)
	if err != nil {
		return nil, fmt.Errorf("connect to rabbitmq: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create rabbitmq channel: %w", err)
	}

	pubCh, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("create rabbitmq publish channel: %w", err)

	}
	if err := pubCh.Confirm(false); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	returns := pubCh.NotifyReturn(make(chan amqp.Return, 1))
	q := &RabbitMQQueue{
		conn:    conn,
		channel: ch,
		pubCh:   pubCh,
		returns: returns,
	}

	if err := q.declareTopology(); err != nil {
		_ = q.Close()
		return nil, err
	}

	return q, nil
}

func ProvideRabbitMQURL(cfg *models.Config) string {
	return cfg.RabbitMQ.URL
}

func ProvideQueue(q *RabbitMQQueue) Queue {
	return q
}

func (q *RabbitMQQueue) declareTopology() error {
	// 1. Declare the dead-letter exchange first
	if err := q.channel.ExchangeDeclare(
		consts.IngestionDeadLetterExchangeName, // name
		"direct",                               // type
		true,                                   // durable
		false,                                  // auto deleted
		false,                                  // internal
		false,                                  // no wait
		nil,                                    // args

	); err != nil {
		return fmt.Errorf("declare dead-letter exchange: %w", err)
	}
	// 2. Declare the dead-letter queue
	dld, err := q.channel.QueueDeclare(
		consts.IngestionDeadLetterQueueName, // name
		true,                                // durable
		false,                               // delete when un used
		false,                               // exclusive
		false,                               // no wait
		nil,                                 // args

	)
	if err != nil {
		return fmt.Errorf("declare dead-letter queue: %w", err)
	}
	// 3. Bind dead-letter queue to dead-letter exchange
	if err := q.channel.QueueBind(
		dld.Name,
		consts.IngestionDeadLetterRoutingKey,
		consts.IngestionDeadLetterExchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("bind dead-letter queue: %w", err)
	}

	// 4. Declare the main exchange
	if err := q.channel.ExchangeDeclare(
		consts.IngestionExchangeName,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare main exchange: %w", err)
	}
	// 5. Declare the main queue, wired to dead-letter on Nack/expiry
	mainQueueArgs := amqp.Table{
		"x-dead-letter-exchange":    consts.IngestionDeadLetterExchangeName,
		"x-dead-letter-routing-key": consts.IngestionDeadLetterRoutingKey,
	}
	mainQueue, err := q.channel.QueueDeclare(
		consts.IngestionQueueName,
		true,          // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		mainQueueArgs, // args — this is what links it to the DLX
	)
	if err != nil {
		return fmt.Errorf("declare main queue: %w", err)
	}
	// 6. Bind main queue to main exchange
	if err := q.channel.QueueBind(
		mainQueue.Name,
		consts.IngestionRoutingKey,
		consts.IngestionExchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("bind main queue: %w", err)
	}
	return q.declareInvitationTopology()
}

func (q *RabbitMQQueue) declareInvitationTopology() error {
	ch := q.channel
	if err := ch.ExchangeDeclare(
		consts.InvitationExchange,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare invitation exchange: %w", err)
	}
	// main queue
	if _, err := ch.QueueDeclare(consts.InvitationQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare invitation queue: %w", err)
	}
	if err := ch.QueueBind(consts.InvitationQueue, consts.InvitationRoutingKey, consts.InvitationExchange, false, nil); err != nil {
		return err
	}

	// retry queue: messages wait here, then dead-letter back to the main queue
	if _, err := ch.QueueDeclare(consts.InvitationRetryQueue, true, false, false, false, amqp.Table{
		"x-message-ttl":             int32(consts.InvitationRetryDelay.Milliseconds()),
		"x-dead-letter-exchange":    consts.InvitationExchange,
		"x-dead-letter-routing-key": consts.InvitationRoutingKey,
	}); err != nil {
		return fmt.Errorf("declare invitation retry queue: %w", err)
	}
	if err := ch.QueueBind(consts.InvitationRetryQueue, consts.InvitationRetryRoutingKey, consts.InvitationExchange, false, nil); err != nil {
		return err
	}

	// dead queue: nobody consumes it, you inspect it in the management UI
	if _, err := ch.QueueDeclare(consts.InvitationDeadQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare invitation dead queue: %w", err)
	}

	if err := ch.QueueBind(consts.InvitationDeadQueue, consts.InvitationDeadRoutingKey, consts.InvitationExchange, false, nil); err != nil {
		return err
	}

	return nil
}

func (q *RabbitMQQueue) publishConfirmed(ctx context.Context, exchange string, routingKey string, message amqp.Publishing) error {
	q.pubMu.Lock()
	defer q.pubMu.Unlock()

	for {
		select {
		case <-q.returns:
			continue
		default:
			goto drained
		}
	}

drained:
	conf, err := q.pubCh.PublishWithDeferredConfirmWithContext(
		ctx,
		exchange,
		routingKey,
		true, // mandatory: report unroutable messages
		false,
		message,
	)
	if err != nil {
		return fmt.Errorf("publish message to %q: %w", routingKey, err)
	}

	// Once publishing starts, wait for the broker's confirmation even if
	// ctx is canceled. Returning early could leave a mandatory-return
	// notification pending and misattribute it to the next serialized publish.
	acked := conf.Wait()
	if !acked {
		return fmt.Errorf("broker did not ack message %q", message.MessageId)
	}

	select {
	case returned, ok := <-q.returns:
		if !ok {
			return fmt.Errorf("rabbitmq return notification channel closed")
		}
		if returned.MessageId == message.MessageId {
			return fmt.Errorf("rabbitmq returned unroutable message %q (reply code %d: %s)", returned.MessageId, returned.ReplyCode, returned.ReplyText)
		}
		return fmt.Errorf("unexpected returned message %q while publishing %q", returned.MessageId, message.MessageId)
	default:
	}

	return nil
}

func (q *RabbitMQQueue) Publish(ctx context.Context, routingKey string, body []byte) error {
	return q.publishConfirmed(
		ctx,
		consts.IngestionExchangeName,
		routingKey,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    uuid.NewString(),
			Body:         body,
		},
	)
}

func (q *RabbitMQQueue) PublishInvitation(ctx context.Context, messageID string, body []byte) error {
	return q.publishConfirmed(
		ctx,
		consts.InvitationExchange,
		consts.InvitationRoutingKey,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    messageID,
			Headers:      amqp.Table{"x-attempt": int32(1)},
			Body:         body,
		},
	)
}

var ErrPermanent = errors.New("permanent failure")

type RetryOptions struct {
	Queue       string
	Exchange    string
	RetryKey    string
	DeadKey     string
	MaxAttempts int
	Concurrency int
}

func (q *RabbitMQQueue) ConsumeWithRetry(ctx context.Context, opts RetryOptions, handler func(ctx context.Context, body []byte) error) error {
	if opts.Queue == "" {
		return errors.New("retry consumer queue must not be empty")
	}
	if opts.Exchange == "" || opts.RetryKey == "" || opts.DeadKey == "" {
		return errors.New("retry exchange and routing keys must not be empty")
	}
	if opts.MaxAttempts < 1 {
		return errors.New("max retry attempts must be at least 1")
	}
	if opts.Concurrency < 1 {
		return errors.New("retry consumer concurrency must be at least 1")
	}

	ch, err := q.conn.Channel()
	if err != nil {
		return fmt.Errorf("open consumer channel: %w", err)
	}

	consumerTag := "retry-" + uuid.NewString()
	if err := ch.Qos(opts.Concurrency, 0, false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(opts.Queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("start consuming queue %q: %w", opts.Queue, err)
	}

	workerCtx, cancelWorkers := context.WithCancel(ctx)
	var wg sync.WaitGroup
	workerErr := make(chan error, 1)
	sem := make(chan struct{}, opts.Concurrency)

	// Stop new deliveries, ask in-flight handlers to stop, and let workers
	// finish while the channel is still open. Closing the channel then
	// requeues anything left unacknowledged.
	defer func() {
		_ = ch.Cancel(consumerTag, false)
		cancelWorkers()
		wg.Wait()
		_ = ch.Close()
	}()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-workerErr:
			return err

		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}

			wg.Add(1)
			go func(d amqp.Delivery) {
				defer wg.Done()
				defer func() { <-sem }()

				if err := processWithRetry(workerCtx, q, opts, d, handler); err != nil {
					select {
					case workerErr <- fmt.Errorf("process delivery %q: %w", d.MessageId, err):
					default:
					}
				}
			}(d)
		}
	}
}

func processWithRetry(ctx context.Context, q *RabbitMQQueue, opts RetryOptions, delivery amqp.Delivery, handler func(ctx context.Context, body []byte) error) error {
	err := handler(ctx, delivery.Body)
	if err == nil {
		if ackErr := delivery.Ack(false); ackErr != nil {
			return fmt.Errorf("ack successful delivery: %w", ackErr)
		}
		return nil
	}

	attempt := 1
	if v, ok := delivery.Headers["x-attempt"].(int32); ok {
		attempt = int(v)
	}

	key := opts.RetryKey
	headers := amqp.Table{
		"x-attempt": int32(attempt + 1),
	}

	if errors.Is(err, ErrPermanent) || attempt >= opts.MaxAttempts {
		key = opts.DeadKey
		headers = amqp.Table{
			"x-attempt":        int32(attempt),
			"x-failure-reason": err.Error(),
		}
	}

	publishErr := q.publishConfirmed(
		ctx,
		opts.Exchange,
		key,
		amqp.Publishing{
			ContentType:  delivery.ContentType,
			DeliveryMode: amqp.Persistent,
			MessageId:    delivery.MessageId,
			Headers:      headers,
			Body:         delivery.Body,
		},
	)
	if publishErr != nil {
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			return fmt.Errorf("publish replacement failed: %v; nack original failed: %w", publishErr, nackErr)
		}
		return fmt.Errorf("publish replacement: %w", publishErr)
	}

	if ackErr := delivery.Ack(false); ackErr != nil {
		return fmt.Errorf("ack original after replacement publish: %w", ackErr)
	}

	return nil
}

func (q *RabbitMQQueue) Consume(ctx context.Context, queueName string, handler func([]byte) error) error {
	deliveries, err := q.channel.Consume(
		queueName,
		"",    // consumer tag (auto-generated)
		false, // auto-ack — false, we ack manually
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("start consuming: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}

			if err := handler(d.Body); err != nil {
				if nackErr := d.Nack(false, false); nackErr != nil {
					return fmt.Errorf(
						"nack failed message: %w (handler error: %v)",
						nackErr,
						err,
					)
				}
				continue
			}

			if err := d.Ack(false); err != nil {
				return fmt.Errorf("ack message: %w", err)
			}
		}
	}
}

func (q *RabbitMQQueue) Close() error {
	var firstErr error

	if q.channel != nil {
		if err := q.channel.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close consumer channel: %w", err)
		}
	}

	if q.pubCh != nil {
		if err := q.pubCh.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close publisher channel: %w", err)
		}
	}

	if q.conn != nil {
		if err := q.conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close rabbitmq connection: %w", err)
		}
	}

	return firstErr
}

func MarshalIngestionJob(jobID, documentID, versionID string) ([]byte, error) {
	return json.Marshal(map[string]string{
		"job_id":      jobID,
		"document_id": documentID,
		"version_id":  versionID,
	})
}

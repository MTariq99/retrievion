package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/mtariq99/retrievion/internal/consts"
	"github.com/mtariq99/retrievion/models"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Queue interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
	Consume(ctx context.Context, queueName string, handler func(models.QueueMessage) error) error
	Ack(ctx context.Context, msg models.QueueMessage) error
	Nack(ctx context.Context, msg models.QueueMessage) error
}

type RabbitMQQueue struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	pubCh   *amqp.Channel
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
	q := &RabbitMQQueue{
		conn:    conn,
		channel: ch,
		pubCh:   pubCh,
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

func (q *RabbitMQQueue) Publish(ctx context.Context, routingKey string, body []byte) error {
	return q.channel.PublishWithContext(
		ctx,
		consts.IngestionExchangeName,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	)
}

func (q *RabbitMQQueue) PublishInvitation(ctx context.Context, messageID string, body []byte) error {
	q.pubMu.Lock()
	defer q.pubMu.Unlock()

	conf, err := q.pubCh.PublishWithDeferredConfirmWithContext(
		ctx,
		consts.InvitationExchange,
		consts.InvitationRoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    messageID,
			Headers:      amqp.Table{"x-attempt": int32(1)},
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish invitation: %w", err)
	}
	acked, err := conf.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait for broker confirm: %w", err)
	}
	if !acked {
		return errors.New("broker nacked invitation message")
	}
	return nil
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
	ch, err := q.conn.Channel()
	if err != nil {
		return fmt.Errorf("open consumer channel: %w", err)
	}

	defer ch.Close()
	if err := ch.Qos(opts.Concurrency, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(consts.InvitationQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("start consuming: %w", err)
	}
	jobCtx := context.WithoutCancel(ctx)
	var wg sync.WaitGroup
	defer wg.Wait()
	sem := make(chan struct{}, opts.Concurrency)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				_ = d.Nack(false, true)
				return ctx.Err()
			}
			wg.Add(1)
			go func(d amqp.Delivery) {
				defer wg.Done()
				defer func() { <-sem }()
				processWithRetry(jobCtx, ch, opts, d, handler)
			}(d)
		}
	}
}

func processWithRetry(ctx context.Context, ch *amqp.Channel, opts RetryOptions, delivery amqp.Delivery, handler func(ctx context.Context, body []byte) error) {
	err := handler(ctx, delivery.Body)
	if err == nil {
		_ = delivery.Ack(false)
		return
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

	pubErr := ch.PublishWithContext(ctx, opts.Exchange, key, false, false, amqp.Publishing{
		ContentType:  delivery.ContentType,
		DeliveryMode: amqp.Persistent,
		MessageId:    delivery.MessageId,
		Headers:      headers,
		Body:         delivery.Body,
	})

	if pubErr != nil {
		_ = delivery.Nack(false, true)
		return
	}

	_ = delivery.Ack(false)
}

func (q *RabbitMQQueue) Consume(ctx context.Context, queueName string, handler func(models.QueueMessage) error) error {
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

			msg := models.QueueMessage{Body: d.Body, DeliveryTag: d.DeliveryTag, Delivery: d}

			if err := handler(msg); err != nil {
				_ = q.Nack(ctx, msg)
				continue
			}

			if err := q.Ack(ctx, msg); err != nil {
				return fmt.Errorf("ack message: %w", err)
			}
		}
	}
}

func (q *RabbitMQQueue) Close() error {
	if q.channel != nil {
		q.channel.Close()
	}
	if q.conn != nil {
		return q.conn.Close()
	}
	if q.pubCh != nil {
		q.pubCh.Close()
	}
	return nil
}

func MarshalIngestionJob(jobID, documentID, versionID string) ([]byte, error) {
	return json.Marshal(map[string]string{
		"job_id":      jobID,
		"document_id": documentID,
		"version_id":  versionID,
	})
}

func (q *RabbitMQQueue) Ack(ctx context.Context, msg models.QueueMessage) error {
	return msg.Delivery.Ack(false)
}

func (q *RabbitMQQueue) Nack(ctx context.Context, msg models.QueueMessage) error {
	return msg.Delivery.Nack(false, false)
}

//go:build integration

package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/models"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestConsumeWithRetryIntegration(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}

	cfg := &models.Config{}
	cfg.RabbitMQ.URL = url

	q, err := NewRabbitMQQueue(cfg)
	if err != nil {
		t.Fatalf("connect to RabbitMQ: %v", err)
	}

	suffix := uuid.NewString()
	exchange := "test.retry." + suffix
	mainQueue := "test.retry.main." + suffix
	retryQueue := "test.retry.delay." + suffix
	deadQueue := "test.retry.dead." + suffix

	mainKey := "main"
	retryKey := "retry"
	deadKey := "dead"

	admin, err := q.conn.Channel()
	if err != nil {
		_ = q.Close()
		t.Fatalf("open topology channel: %v", err)
	}

	// This test owns only these uniquely named resources.
	defer func() {
		_, _ = admin.QueueDelete(mainQueue, false, false, false)
		_, _ = admin.QueueDelete(retryQueue, false, false, false)
		_, _ = admin.QueueDelete(deadQueue, false, false, false)
		_ = admin.ExchangeDelete(exchange, false, false)
		_ = admin.Close()
		_ = q.Close()
	}()

	if err := admin.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		t.Fatalf("declare test exchange: %v", err)
	}

	if _, err := admin.QueueDeclare(mainQueue, true, false, false, false, nil); err != nil {
		t.Fatalf("declare main queue: %v", err)
	}
	if _, err := admin.QueueDeclare(retryQueue, true, false, false, false, amqp.Table{
		"x-message-ttl":             int32(200),
		"x-dead-letter-exchange":    exchange,
		"x-dead-letter-routing-key": mainKey,
	}); err != nil {
		t.Fatalf("declare retry queue: %v", err)
	}
	if _, err := admin.QueueDeclare(deadQueue, true, false, false, false, nil); err != nil {
		t.Fatalf("declare dead queue: %v", err)
	}

	for _, binding := range []struct {
		queue string
		key   string
	}{
		{mainQueue, mainKey},
		{retryQueue, retryKey},
		{deadQueue, deadKey},
	} {
		if err := admin.QueueBind(binding.queue, binding.key, exchange, false, nil); err != nil {
			t.Fatalf("bind %q: %v", binding.queue, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var attempts atomic.Int32
	succeeded := make(chan struct{})
	consumerDone := make(chan error, 1)

	go func() {
		consumerDone <- q.ConsumeWithRetry(ctx, RetryOptions{
			Queue:       mainQueue,
			Exchange:    exchange,
			RetryKey:    retryKey,
			DeadKey:     deadKey,
			MaxAttempts: 3,
			Concurrency: 1,
		}, func(ctx context.Context, body []byte) error {
			attempt := attempts.Add(1)
			if string(body) != "retry-integration-test" {
				return fmt.Errorf("unexpected body: %q", body)
			}

			if attempt == 1 {
				return errors.New("intentional first-attempt failure")
			}

			if attempt == 2 {
				close(succeeded)
			}
			return nil
		})
	}()

	messageID := uuid.NewString()
	if err := q.publishConfirmed(ctx, exchange, mainKey, amqp.Publishing{
		ContentType:  "text/plain",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Headers:      amqp.Table{"x-attempt": int32(1)},
		Body:         []byte("retry-integration-test"),
	}); err != nil {
		t.Fatalf("publish test message: %v", err)
	}

	select {
	case <-succeeded:
		// The handler succeeded on the second attempt.
	case err := <-consumerDone:
		t.Fatalf("consumer exited before retry succeeded: %v", err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for retry; attempts=%d", attempts.Load())
	}

	if got := attempts.Load(); got != 2 {
		t.Fatalf("handler attempts = %d, want 2", got)
	}

	cancel()
	select {
	case err := <-consumerDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("consumer shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("consumer did not stop after cancellation")
	}

	mainState, err := admin.QueueInspect(mainQueue)
	if err != nil {
		t.Fatalf("inspect main queue: %v", err)
	}
	if mainState.Messages != 0 {
		t.Errorf("main queue has %d messages, want 0", mainState.Messages)
	}

	retryState, err := admin.QueueInspect(retryQueue)
	if err != nil {
		t.Fatalf("inspect retry queue: %v", err)
	}
	if retryState.Messages != 0 {
		t.Errorf("retry queue has %d messages, want 0", retryState.Messages)
	}
}

func TestConsumeWithRetryDeadLettersAfterMaxAttempts(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}

	cfg := &models.Config{}
	cfg.RabbitMQ.URL = url

	q, err := NewRabbitMQQueue(cfg)
	if err != nil {
		t.Fatalf("connect to RabbitMQ: %v", err)
	}

	suffix := uuid.NewString()
	exchange := "test.dead." + suffix
	mainQueue := "test.dead.main." + suffix
	retryQueue := "test.dead.retry." + suffix
	deadQueue := "test.dead.queue." + suffix

	mainKey, retryKey, deadKey := "main", "retry", "dead"

	admin, err := q.conn.Channel()
	if err != nil {
		_ = q.Close()
		t.Fatalf("open topology channel: %v", err)
	}

	defer func() {
		_, _ = admin.QueueDelete(mainQueue, false, false, false)
		_, _ = admin.QueueDelete(retryQueue, false, false, false)
		_, _ = admin.QueueDelete(deadQueue, false, false, false)
		_ = admin.ExchangeDelete(exchange, false, false)
		_ = admin.Close()
		_ = q.Close()
	}()

	if err := admin.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		t.Fatalf("declare test exchange: %v", err)
	}
	if _, err := admin.QueueDeclare(mainQueue, true, false, false, false, nil); err != nil {
		t.Fatalf("declare main queue: %v", err)
	}
	if _, err := admin.QueueDeclare(retryQueue, true, false, false, false, amqp.Table{
		"x-message-ttl":             int32(100),
		"x-dead-letter-exchange":    exchange,
		"x-dead-letter-routing-key": mainKey,
	}); err != nil {
		t.Fatalf("declare retry queue: %v", err)
	}
	if _, err := admin.QueueDeclare(deadQueue, true, false, false, false, nil); err != nil {
		t.Fatalf("declare dead queue: %v", err)
	}

	for _, binding := range []struct {
		queue string
		key   string
	}{
		{mainQueue, mainKey},
		{retryQueue, retryKey},
		{deadQueue, deadKey},
	} {
		if err := admin.QueueBind(binding.queue, binding.key, exchange, false, nil); err != nil {
			t.Fatalf("bind %q: %v", binding.queue, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var attempts atomic.Int32
	consumerDone := make(chan error, 1)

	go func() {
		consumerDone <- q.ConsumeWithRetry(ctx, RetryOptions{
			Queue:       mainQueue,
			Exchange:    exchange,
			RetryKey:    retryKey,
			DeadKey:     deadKey,
			MaxAttempts: 3,
			Concurrency: 1,
		}, func(context.Context, []byte) error {
			attempts.Add(1)
			return errors.New("intentional terminal failure")
		})
	}()

	messageID := uuid.NewString()
	if err := q.publishConfirmed(ctx, exchange, mainKey, amqp.Publishing{
		ContentType:  "text/plain",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Headers:      amqp.Table{"x-attempt": int32(1)},
		Body:         []byte("dead-letter-integration-test"),
	}); err != nil {
		t.Fatalf("publish test message: %v", err)
	}

	var deadMessage amqp.Delivery
	deadDeliveries, err := admin.Consume(deadQueue, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("consume dead-letter queue: %v", err)
	}

	select {
	case deadMessage = <-deadDeliveries:
		if deadMessage.MessageId != messageID {
			t.Errorf("dead-letter message ID = %q, want %q", deadMessage.MessageId, messageID)
		}
		if got := deadMessage.Headers["x-attempt"]; got != int32(3) {
			t.Errorf("dead-letter x-attempt = %v, want 3", got)
		}
		if got := deadMessage.Headers["x-failure-reason"]; got != "intentional terminal failure" {
			t.Errorf("dead-letter failure reason = %v", got)
		}
	case err := <-consumerDone:
		t.Fatalf("consumer exited before dead-letter routing: %v", err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for dead letter; handler attempts=%d", attempts.Load())
	}

	if got := attempts.Load(); got != 3 {
		t.Errorf("handler attempts = %d, want 3", got)
	}

	cancel()
	select {
	case err := <-consumerDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("consumer shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("consumer did not stop after cancellation")
	}

	mainState, err := admin.QueueInspect(mainQueue)
	if err != nil {
		t.Fatalf("inspect main queue: %v", err)
	}
	if mainState.Messages != 0 {
		t.Errorf("main queue has %d messages, want 0", mainState.Messages)
	}

	retryState, err := admin.QueueInspect(retryQueue)
	if err != nil {
		t.Fatalf("inspect retry queue: %v", err)
	}
	if retryState.Messages != 0 {
		t.Errorf("retry queue has %d messages, want 0", retryState.Messages)
	}
}

func TestConsumeWithRetryFailsFastWhenRetryPublishFails(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}

	cfg := &models.Config{}
	cfg.RabbitMQ.URL = url

	q, err := NewRabbitMQQueue(cfg)
	if err != nil {
		t.Fatalf("connect to RabbitMQ: %v", err)
	}

	suffix := uuid.NewString()
	exchange := "test.retry-publish-failure." + suffix
	mainQueue := "test.retry-publish-failure.main." + suffix
	mainKey := "main"
	unboundRetryKey := "retry-unbound"
	deadKey := "dead-unbound"

	admin, err := q.conn.Channel()
	if err != nil {
		_ = q.Close()
		t.Fatalf("open topology channel: %v", err)
	}

	defer func() {
		_, _ = admin.QueueDelete(mainQueue, false, false, false)
		_ = admin.ExchangeDelete(exchange, false, false)
		_ = admin.Close()
		_ = q.Close()
	}()

	if err := admin.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		t.Fatalf("declare test exchange: %v", err)
	}
	if _, err := admin.QueueDeclare(mainQueue, true, false, false, false, nil); err != nil {
		t.Fatalf("declare main queue: %v", err)
	}
	if err := admin.QueueBind(mainQueue, mainKey, exchange, false, nil); err != nil {
		t.Fatalf("bind main queue: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var attempts atomic.Int32
	consumerDone := make(chan error, 1)

	go func() {
		consumerDone <- q.ConsumeWithRetry(ctx, RetryOptions{
			Queue:       mainQueue,
			Exchange:    exchange,
			RetryKey:    unboundRetryKey,
			DeadKey:     deadKey,
			MaxAttempts: 3,
			Concurrency: 1,
		}, func(context.Context, []byte) error {
			attempts.Add(1)
			return errors.New("intentional handler failure")
		})
	}()

	messageID := uuid.NewString()
	if err := q.publishConfirmed(ctx, exchange, mainKey, amqp.Publishing{
		ContentType:  "text/plain",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Headers:      amqp.Table{"x-attempt": int32(1)},
		Body:         []byte("retry-publish-failure-test"),
	}); err != nil {
		t.Fatalf("publish test message: %v", err)
	}

	select {
	case err := <-consumerDone:
		if err == nil {
			t.Fatal("consumer returned nil; expected retry publication failure")
		}
		if !strings.Contains(err.Error(), "publish replacement") {
			t.Fatalf("consumer error = %v, want retry replacement publication failure", err)
		}
	case <-ctx.Done():
		t.Fatalf("consumer did not fail fast; handler attempts=%d", attempts.Load())
	}

	if got := attempts.Load(); got != 1 {
		t.Errorf("handler attempts = %d, want exactly 1", got)
	}

	state, err := admin.QueueInspect(mainQueue)
	if err != nil {
		t.Fatalf("inspect main queue: %v", err)
	}
	if state.Messages != 1 {
		t.Errorf("main queue has %d messages, want original delivery requeued (1)", state.Messages)
	}
}

func TestConsumeWithRetryCancellationRequeuesInflight(t *testing.T) {
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL not set")
	}

	cfg := &models.Config{}
	cfg.RabbitMQ.URL = url

	q, err := NewRabbitMQQueue(cfg)
	if err != nil {
		t.Fatalf("connect to RabbitMQ: %v", err)
	}

	suffix := uuid.NewString()
	exchange := "test.cancel." + suffix
	mainQueue := "test.cancel.main." + suffix

	admin, err := q.conn.Channel()
	if err != nil {
		_ = q.Close()
		t.Fatalf("open topology channel: %v", err)
	}

	defer func() {
		_, _ = admin.QueueDelete(mainQueue, false, false, false)
		_ = admin.ExchangeDelete(exchange, false, false)
		_ = admin.Close()
		_ = q.Close()
	}()

	if err := admin.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		t.Fatalf("declare exchange: %v", err)
	}
	if _, err := admin.QueueDeclare(mainQueue, true, false, false, false, nil); err != nil {
		t.Fatalf("declare queue: %v", err)
	}
	if err := admin.QueueBind(mainQueue, "main", exchange, false, nil); err != nil {
		t.Fatalf("bind queue: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{}, 2)
	consumerDone := make(chan error, 1)

	go func() {
		consumerDone <- q.ConsumeWithRetry(ctx, RetryOptions{
			Queue:       mainQueue,
			Exchange:    exchange,
			RetryKey:    "retry-unbound",
			DeadKey:     "dead-unbound",
			MaxAttempts: 3,
			Concurrency: 2,
		}, func(ctx context.Context, _ []byte) error {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	for i := 0; i < 2; i++ {
		if err := q.publishConfirmed(ctx, exchange, "main", amqp.Publishing{
			ContentType:  "text/plain",
			DeliveryMode: amqp.Persistent,
			MessageId:    uuid.NewString(),
			Body:         []byte(fmt.Sprintf("cancel-test-%d", i)),
		}); err != nil {
			t.Fatalf("publish message %d: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case err := <-consumerDone:
			t.Fatalf("consumer exited before both handlers started: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for both handlers to start")
		}
	}

	cancel()

	select {
	case err := <-consumerDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("consumer returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not shut down after cancellation")
	}

	state, err := admin.QueueInspect(mainQueue)
	if err != nil {
		t.Fatalf("inspect main queue: %v", err)
	}
	if state.Messages != 2 {
		t.Errorf("main queue has %d messages, want 2 requeued deliveries", state.Messages)
	}
}

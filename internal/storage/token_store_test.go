package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/mtariq99/retrievion/internal/errs"
	"github.com/mtariq99/retrievion/models"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*RedisClient, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = c.Close() })
	return &RedisClient{client: c}, mr
}

func saveTestToken(t *testing.T, r *RedisClient, id string, ttl time.Duration) {
	t.Helper()
	err := r.SaveToken(context.Background(), &models.OpaqueToken{
		TokenID:   id,
		UserID:    uuid.New(),
		TokenType: "refresh",
		ExpiresAt: time.Now().Add(ttl),
	})
	if err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
}

func TestConsumeToken_ConcurrentSingleWinner(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()
	saveTestToken(t, r, "tok-1", time.Hour)

	const n = 50
	var (
		wg        sync.WaitGroup
		start     = make(chan struct{})
		wins      atomic.Int32
		consumed  atomic.Int32
		unexpectd atomic.Int32
	)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all goroutines at once
			tok, err := r.ConsumeToken(ctx, "tok-1")
			switch {
			case err == nil && tok != nil && tok.TokenID == "tok-1":
				wins.Add(1)
			case errors.Is(err, errs.ErrTokenAlreadyConsumed):
				consumed.Add(1)
			default:
				unexpectd.Add(1)
				t.Errorf("unexpected result: tok=%v err=%v", tok, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := wins.Load(); got != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", got)
	}
	if got := consumed.Load(); got != n-1 {
		t.Fatalf("expected %d ErrTokenAlreadyConsumed, got %d", n-1, got)
	}
}

func TestConsumeToken_MissingToken(t *testing.T) {
	r, _ := newTestRedis(t)
	_, err := r.ConsumeToken(context.Background(), "does-not-exist")
	if !errors.Is(err, errs.ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestConsumeToken_ExpiredToken(t *testing.T) {
	r, mr := newTestRedis(t)
	saveTestToken(t, r, "tok-exp", 2*time.Second)

	mr.FastForward(3 * time.Second)

	_, err := r.ConsumeToken(context.Background(), "tok-exp")
	if !errors.Is(err, errs.ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound for expired token, got %v", err)
	}
}

func TestConsumeToken_SecondCallRejected(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()
	saveTestToken(t, r, "tok-2", time.Hour)

	if _, err := r.ConsumeToken(ctx, "tok-2"); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if _, err := r.ConsumeToken(ctx, "tok-2"); !errors.Is(err, errs.ErrTokenAlreadyConsumed) {
		t.Fatalf("second consume: expected ErrTokenAlreadyConsumed, got %v", err)
	}
	if _, ok, _ := r.ConsumedAt(ctx, "tok-2"); !ok {
		t.Fatal("ConsumedAt should report the token as consumed")
	}
}

func TestReleaseToken_AllowsRetry(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()
	saveTestToken(t, r, "tok-3", time.Hour)

	if _, err := r.ConsumeToken(ctx, "tok-3"); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if err := r.ReleaseToken(ctx, "tok-3"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := r.ConsumeToken(ctx, "tok-3"); err != nil {
		t.Fatalf("consume after release: %v", err)
	}
}

func TestConsumeToken_MarkerTTLMatchesToken(t *testing.T) {
	r, mr := newTestRedis(t)
	saveTestToken(t, r, "tok-4", time.Hour)

	if _, err := r.ConsumeToken(context.Background(), "tok-4"); err != nil {
		t.Fatalf("consume: %v", err)
	}

	ttl := mr.TTL(consumedKey("tok-4"))
	if ttl <= 0 || ttl > time.Hour {
		t.Fatalf("marker TTL should be in (0, 1h], got %v", ttl)
	}

	// Once the token expires, the marker must expire with it.
	mr.FastForward(time.Hour + time.Second)
	if mr.Exists(consumedKey("tok-4")) {
		t.Fatal("marker should have expired with the token")
	}
}

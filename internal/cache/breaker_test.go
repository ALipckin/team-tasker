package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBreaker(t *testing.T) {
	now := time.Unix(0, 0)
	b := newBreaker(2, time.Second)
	b.now = func() time.Time { return now }

	ctx := context.Background()
	fail := errors.New("boom")
	failing := func() error { return fail }
	ok := func() error { return nil }

	_ = b.do(ctx, failing)
	_ = b.do(ctx, failing)

	if err := b.do(ctx, ok); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open circuit, got %v", err)
	}

	now = now.Add(2 * time.Second)

	if err := b.do(ctx, ok); err != nil {
		t.Fatalf("half-open probe should pass, got %v", err)
	}

	if err := b.do(ctx, ok); err != nil {
		t.Fatalf("breaker should be closed, got %v", err)
	}
}

func TestBreakerIgnoresContextErrors(t *testing.T) {
	now := time.Unix(0, 0)
	b := newBreaker(2, time.Second)
	b.now = func() time.Time { return now }

	ctx := context.Background()
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	canceled := func() error { return context.Canceled }

	for range 5 {
		_ = b.do(canceledCtx, canceled)
	}
	if err := b.do(ctx, func() error { return nil }); err != nil {
		t.Fatalf("context errors must not open the breaker, got %v", err)
	}

	fail := errors.New("boom")
	_ = b.do(ctx, func() error { return fail })
	_ = b.do(ctx, func() error { return fail })

	now = now.Add(2 * time.Second)
	_ = b.do(canceledCtx, canceled) // пробный запрос отменён клиентом
	if err := b.do(ctx, func() error { return nil }); err != nil {
		t.Fatalf("next request should become the probe, got %v", err)
	}
}

// Таймаут dial в net удовлетворяет errors.Is(err, context.DeadlineExceeded),
// но при живом ctx клиента это отказ Redis и должен размыкать breaker.
func TestBreakerCountsTimeoutsWithLiveContext(t *testing.T) {
	b := newBreaker(2, time.Second)

	ctx := context.Background()
	timeout := func() error { return context.DeadlineExceeded }

	_ = b.do(ctx, timeout)
	_ = b.do(ctx, timeout)

	if err := b.do(ctx, func() error { return nil }); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open circuit, got %v", err)
	}
}

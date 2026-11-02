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

	for range 2 {
		if err := b.do(ctx, failing); !errors.Is(err, fail) {
			t.Fatalf("failing request = %v, want %v", err, fail)
		}
	}

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
		if err := b.do(canceledCtx, canceled); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled request = %v, want context.Canceled", err)
		}
	}
	if err := b.do(ctx, func() error { return nil }); err != nil {
		t.Fatalf("context errors must not open the breaker, got %v", err)
	}

	fail := errors.New("boom")
	for range 2 {
		if err := b.do(ctx, func() error { return fail }); !errors.Is(err, fail) {
			t.Fatalf("failing request = %v, want %v", err, fail)
		}
	}

	now = now.Add(2 * time.Second)
	if err := b.do(canceledCtx, canceled); !errors.Is(err, context.Canceled) { // пробный запрос отменён клиентом
		t.Fatalf("canceled probe = %v, want context.Canceled", err)
	}
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

	for range 2 {
		if err := b.do(ctx, timeout); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout request = %v, want context.DeadlineExceeded", err)
		}
	}

	if err := b.do(ctx, func() error { return nil }); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open circuit, got %v", err)
	}
}

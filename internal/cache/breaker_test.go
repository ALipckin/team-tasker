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

	fail := errors.New("boom")
	failing := func() error { return fail }
	ok := func() error { return nil }

	_ = b.do(failing)
	_ = b.do(failing)

	if err := b.do(ok); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open circuit, got %v", err)
	}

	now = now.Add(2 * time.Second)

	if err := b.do(ok); err != nil {
		t.Fatalf("half-open probe should pass, got %v", err)
	}

	if err := b.do(ok); err != nil {
		t.Fatalf("breaker should be closed, got %v", err)
	}
}

func TestBreakerIgnoresContextErrors(t *testing.T) {
	now := time.Unix(0, 0)
	b := newBreaker(2, time.Second)
	b.now = func() time.Time { return now }

	canceled := func() error { return context.Canceled }

	for range 5 {
		_ = b.do(canceled)
	}
	if err := b.do(func() error { return nil }); err != nil {
		t.Fatalf("context errors must not open the breaker, got %v", err)
	}

	fail := errors.New("boom")
	_ = b.do(func() error { return fail })
	_ = b.do(func() error { return fail })

	now = now.Add(2 * time.Second)
	_ = b.do(canceled) // пробный запрос отменён клиентом
	if err := b.do(func() error { return nil }); err != nil {
		t.Fatalf("next request should become the probe, got %v", err)
	}
}

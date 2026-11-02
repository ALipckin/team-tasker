package cache

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrCircuitOpen — circuit breaker разомкнут, запрос не выполнялся.
var ErrCircuitOpen = errors.New("circuit breaker is open")

type breakerState int

const (
	stateClosed breakerState = iota
	stateOpen
	stateHalfOpen
)

// breaker — простой circuit breaker.
// После maxFailures ошибок подряд размыкается на openTimeout,
// затем пропускает один пробный запрос (half-open).
type breaker struct {
	mu          sync.Mutex
	state       breakerState
	failures    int
	maxFailures int
	openTimeout time.Duration
	openedAt    time.Time
	now         func() time.Time
}

func newBreaker(maxFailures int, openTimeout time.Duration) *breaker {
	return &breaker{
		maxFailures: maxFailures,
		openTimeout: openTimeout,
		now:         time.Now,
	}
}

func (b *breaker) do(ctx context.Context, fn func() error) error {
	if !b.allow() {
		return ErrCircuitOpen
	}

	err := fn()
	b.record(ctx, err)

	return err
}

func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateClosed:
		return true
	case stateOpen:
		if b.now().Sub(b.openedAt) < b.openTimeout {
			return false
		}
		b.state = stateHalfOpen
		return true
	case stateHalfOpen:
		// Пробный запрос уже выполняется.
		return false
	}
	return false
}

func (b *breaker) record(ctx context.Context, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Отмена или таймаут запроса клиента ничего не говорит о здоровье Redis.
	// Смотрим именно на ctx клиента, а не на тип ошибки: таймаут dial в net
	// тоже удовлетворяет errors.Is(err, context.DeadlineExceeded).
	if err != nil && ctx.Err() != nil {
		if b.state == stateHalfOpen {
			// Пробный запрос не дал ответа — следующий запрос станет новым пробным.
			b.state = stateOpen
		}
		return
	}

	if err == nil {
		if b.state != stateClosed {
			slog.Info("circuit breaker closed")
		}
		b.state = stateClosed
		b.failures = 0
		return
	}

	b.failures++
	if b.state == stateHalfOpen || b.failures >= b.maxFailures {
		if b.state != stateOpen {
			slog.Warn("circuit breaker opened", "err", err)
		}
		b.state = stateOpen
		b.openedAt = b.now()
	}
}

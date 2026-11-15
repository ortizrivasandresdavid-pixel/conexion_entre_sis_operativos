package login

import (
	"sync"
	"time"
)

const (
	initialAuthBackoff = time.Second
	maxAuthBackoff     = time.Minute
)

type attemptState struct {
	failures     int
	blockedUntil time.Time
}

type AttemptLimiter struct {
	mu       sync.Mutex
	attempts map[string]attemptState
	now      func() time.Time
}

func NewAttemptLimiter() *AttemptLimiter {
	return &AttemptLimiter{
		attempts: make(map[string]attemptState),
		now:      time.Now,
	}
}

func (l *AttemptLimiter) Check(key string, maxAttempts int) (int, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.currentTime()
	state, exists := l.attempts[key]
	if !exists {
		return maxAttempts, 0
	}
	if now.Before(state.blockedUntil) {
		return remainingAttempts(state.failures, maxAttempts), state.blockedUntil.Sub(now)
	}
	if state.failures >= maxAttempts {
		delete(l.attempts, key)
		return maxAttempts, 0
	}
	return remainingAttempts(state.failures, maxAttempts), 0
}

func (l *AttemptLimiter) RecordFailure(key string, maxAttempts int) (int, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.currentTime()
	state := l.attempts[key]
	if now.Before(state.blockedUntil) {
		return remainingAttempts(state.failures, maxAttempts), state.blockedUntil.Sub(now)
	}
	if state.failures >= maxAttempts {
		state = attemptState{}
	}
	state.failures++
	backoff := authBackoff(state.failures)
	state.blockedUntil = now.Add(backoff)
	l.attempts[key] = state
	return remainingAttempts(state.failures, maxAttempts), backoff
}

func (l *AttemptLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (l *AttemptLimiter) currentTime() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

func remainingAttempts(failures, maxAttempts int) int {
	remaining := maxAttempts - failures
	if remaining < 0 {
		return 0
	}
	return remaining
}

func authBackoff(failures int) time.Duration {
	delay := initialAuthBackoff
	for range max(failures-1, 0) {
		if delay >= maxAuthBackoff/2 {
			return maxAuthBackoff
		}
		delay *= 2
	}
	return delay
}

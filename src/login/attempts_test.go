package login

import (
	"testing"
	"time"
)

func TestAttemptLimiterPersistsFailuresAndBackoff(t *testing.T) {
	limiter := NewAttemptLimiter()
	now := time.Unix(100, 0)
	limiter.now = func() time.Time { return now }

	remaining, delay := limiter.RecordFailure("192.0.2.10", 3)
	if remaining != 2 || delay != time.Second {
		t.Fatalf("first failure = (%d, %s), want (2, 1s)", remaining, delay)
	}
	remaining, retry := limiter.Check("192.0.2.10", 3)
	if remaining != 2 || retry != time.Second {
		t.Fatalf("blocked check = (%d, %s), want (2, 1s)", remaining, retry)
	}

	now = now.Add(delay)
	remaining, retry = limiter.Check("192.0.2.10", 3)
	if remaining != 2 || retry != 0 {
		t.Fatalf("check after backoff = (%d, %s), want (2, 0)", remaining, retry)
	}
	remaining, delay = limiter.RecordFailure("192.0.2.10", 3)
	if remaining != 1 || delay != 2*time.Second {
		t.Fatalf("second failure = (%d, %s), want (1, 2s)", remaining, delay)
	}

	now = now.Add(delay)
	remaining, delay = limiter.RecordFailure("192.0.2.10", 3)
	if remaining != 0 || delay != 4*time.Second {
		t.Fatalf("third failure = (%d, %s), want (0, 4s)", remaining, delay)
	}
	remaining, retry = limiter.Check("192.0.2.10", 3)
	if remaining != 0 || retry != 4*time.Second {
		t.Fatalf("locked check = (%d, %s), want (0, 4s)", remaining, retry)
	}

	now = now.Add(delay)
	remaining, retry = limiter.Check("192.0.2.10", 3)
	if remaining != 3 || retry != 0 {
		t.Fatalf("check after lockout = (%d, %s), want (3, 0)", remaining, retry)
	}
}

func TestAttemptLimiterResetsOnSuccess(t *testing.T) {
	limiter := NewAttemptLimiter()
	limiter.RecordFailure("192.0.2.10", 3)
	limiter.RecordSuccess("192.0.2.10")

	remaining, retry := limiter.Check("192.0.2.10", 3)
	if remaining != 3 || retry != 0 {
		t.Fatalf("check after success = (%d, %s), want (3, 0)", remaining, retry)
	}
}

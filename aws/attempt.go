package aws

import (
	"time"
)

// AttemptStrategy represents a strategy for waiting for an action
// to complete successfully.
type AttemptStrategy interface {
	// Start begins a new sequence of attempts for the strategy.
	Start() Attempt
}

// Attempt represents a sequence of attempts to perform an action successfully.
type Attempt interface {
	// Next waits until it is time to perform the next attempt or returns
	// false if it is time to stop trying. If error is provided it is the error
	// the previous attempt in the sequence failed with, or nil if this is the
	// first attempt.
	Next(err error) bool
	// HasNext returns whether another attempt will be made if the current
	// one fails. If it returns true, the following call to Next is
	// guaranteed to return true.
	HasNext() bool
}

// FixedAttemptStrategy implements AttemptStrategy and applies a fixed backoff
// between attempts, optionally performing a minimum number of attempts before
// abandoning the attempt sequence once a fixed time has passed.
type FixedAttemptStrategy struct {
	Total time.Duration // total duration of attempt.
	Delay time.Duration // interval between each try in the burst.
	Min   int           // minimum number of retries; overrides Total
}

// FixedAttempt implements the Attempt interface for the FixedAttemptStrategy.
type FixedAttempt struct {
	strategy FixedAttemptStrategy
	last     time.Time
	end      time.Time
	force    bool
	count    int
}

// Start begins a new sequence of attempts for the given strategy.
func (s FixedAttemptStrategy) Start() Attempt {
	now := time.Now()
	return &FixedAttempt{
		strategy: s,
		last:     now,
		end:      now.Add(s.Total),
		force:    true,
	}
}

// Next waits until it is time to perform the next attempt or returns
// false if it is time to stop trying.
func (a *FixedAttempt) Next(err error) bool {
	now := time.Now()
	sleep := a.nextSleep(now)
	if !a.force && !now.Add(sleep).Before(a.end) && a.strategy.Min <= a.count {
		return false
	}
	a.force = false
	if sleep > 0 && a.count > 0 {
		time.Sleep(sleep)
		now = time.Now()
	}
	a.count++
	a.last = now
	return true
}

func (a *FixedAttempt) nextSleep(now time.Time) time.Duration {
	sleep := a.strategy.Delay - now.Sub(a.last)
	if sleep < 0 {
		return 0
	}
	return sleep
}

// HasNext returns whether another attempt will be made if the current
// one fails. If it returns true, the following call to Next is
// guaranteed to return true.
func (a *FixedAttempt) HasNext() bool {
	if a.force || a.strategy.Min > a.count {
		return true
	}
	now := time.Now()
	if now.Add(a.nextSleep(now)).Before(a.end) {
		a.force = true
		return true
	}
	return false
}

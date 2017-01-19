package aws_test

import (
	"time"

	"github.com/hughe/goamz/aws"
	. "gopkg.in/check.v1"
)

func (S) TestAttemptTiming(c *C) {
	testAttempt := aws.FixedAttemptStrategy{
		Total: 0.25e9,
		Delay: 0.1e9,
	}
	want := []time.Duration{0, 0.1e9, 0.2e9, 0.2e9}
	got := make([]time.Duration, 0, len(want)) // avoid allocation when testing timing
	t0 := time.Now()
	for a := testAttempt.Start(); a.Next(nil); {
		got = append(got, time.Now().Sub(t0))
	}
	got = append(got, time.Now().Sub(t0))
	c.Assert(got, HasLen, len(want))
	const margin = 0.01e9
	for i, got := range want {
		lo := want[i] - margin
		hi := want[i] + margin
		if got < lo || got > hi {
			c.Errorf("attempt %d want %g got %g", i, want[i].Seconds(), got.Seconds())
		}
	}
}

func (S) TestAttemptNextHasNext(c *C) {
	a := aws.FixedAttemptStrategy{}.Start()
	c.Assert(a.Next(nil), Equals, true)
	c.Assert(a.Next(nil), Equals, false)

	a = aws.FixedAttemptStrategy{}.Start()
	c.Assert(a.Next(nil), Equals, true)
	c.Assert(a.HasNext(), Equals, false)
	c.Assert(a.Next(nil), Equals, false)

	a = aws.FixedAttemptStrategy{Total: 2e8}.Start()
	c.Assert(a.Next(nil), Equals, true)
	c.Assert(a.HasNext(), Equals, true)
	time.Sleep(2e8)
	c.Assert(a.HasNext(), Equals, true)
	c.Assert(a.Next(nil), Equals, true)
	c.Assert(a.Next(nil), Equals, false)

	a = aws.FixedAttemptStrategy{Total: 1e8, Min: 2}.Start()
	time.Sleep(1e8)
	c.Assert(a.Next(nil), Equals, true)
	c.Assert(a.HasNext(), Equals, true)
	c.Assert(a.Next(nil), Equals, true)
	c.Assert(a.HasNext(), Equals, false)
	c.Assert(a.Next(nil), Equals, false)
}

package s3_test

import (
	"testing"
	"time"

	"github.com/hughe/goamz/s3"
)

func TestParseRestoreHeader(t *testing.T) {

	tab := []struct {
		sample string

		success   bool
		ongoing   bool
		expirySet bool
		expiry    time.Time
	}{
		{
			sample:    `ongoing-request="false", expiry-date="Fri, 23 Dec 2012 00:00:00 GMT"`,
			success:   true,
			ongoing:   false,
			expirySet: true,
			expiry:    time.Date(2012, 12, 23, 0, 0, 0, 0, time.UTC),
		},

		{
			sample:    `ongoing-request="true"`,
			success:   true,
			ongoing:   true,
			expirySet: false,
		},
	}

	for i, x := range tab {
		r := s3.RestoreStatus{}
		err := r.ParseAmzRestore(x.sample)

		if x.success {
			if err != nil {
				t.Errorf("%d  got %#v want %#v", i, err, nil)
			}

			if r.OnGoingRequest != x.ongoing {
				t.Errorf("%d OnGoingRequest got %#v want %#v", i, r.OnGoingRequest, x.ongoing)
			}
			if x.expirySet {
				if !r.ExpiryDate.Equal(x.expiry) {
					t.Errorf("%d ExpiryDate got %s want %s", i, r.ExpiryDate, x.expiry)
				}
			} else {
				if !r.ExpiryDate.IsZero() {
					t.Errorf("%d ExpiryDate got %s wanted zero", i, r.ExpiryDate)
				}
			}
		} else {
			if err == nil {
				t.Errorf("%d Failure  got nil want an error", i)
			}
		}

	}

}

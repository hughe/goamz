package s3

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

type GlacierJobParameters struct {
	Tier string `xml:"Tier"`
}

type RestoreRequest struct {
	XMLName xml.Name `xml:"RestoreRequest"`

	Days uint `xml:"Days"`

	GlacierJobParameters GlacierJobParameters
}

type Tier int

const (
	Standard Tier = iota
	Expedited
	Bulk
)

func (t Tier) String() string {
	switch t {
	case Standard:
		return "Standard"
	case Expedited:
		return "Expedited"
	case Bulk:
		return "Bulk"
	}
	panic(fmt.Sprintf("Unknown value for Tier: %d", t))
}

func (b *Bucket) RestoreObject(key string, days uint, tier Tier) (started bool, err error) {
	if !b.S3.v4sign {
		return false, errors.New("SigV4 only")
	}

	rr := RestoreRequest{
		Days: days,
		GlacierJobParameters: GlacierJobParameters{
			Tier: tier.String(),
		},
	}

	data, err := xml.Marshal(&rr)
	if err != nil {
		return false, err
	}

	params := map[string][]string{
		"restore": {""},
	}

	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method:  "POST",
			bucket:  b.Name,
			path:    key,
			params:  params,
			payload: bytes.NewReader(data),
		}
		var statusCode int
		statusCode, err = b.S3.queryWithStatus(req, nil)
		if ShouldRetry(err) && attempt.HasNext() {
			continue
		}
		return statusCode == http.StatusAccepted, err
	}
	panic("unreachable")
}

type StorageClass string

const (
	STANDARD    = "STANDARD"
	STANDARD_IA = "STANDARD_IA"
	GLACIER     = "GLACIER"
)

type RestoreStatus struct {
	OnGoingRequest bool
	ExpiryDate     time.Time
	StorageClass   string
}

func (r RestoreStatus) IsBeingRestored() bool {
	return r.OnGoingRequest
}

func (r RestoreStatus) HasBeenRestored() bool {
	if !r.OnGoingRequest {
		return r.ExpiryDate.IsZero()
	}
	return false
}

var restoreRe = regexp.MustCompile(`ongoing-request="(true|false)"(, expiry-date="([^"]*)")?`)

func (r *RestoreStatus) ParseAmzRestore(s string) (err error) {
	c := restoreRe.Copy()
	parts := c.FindStringSubmatch(s)
	if parts == nil {
		return fmt.Errorf("Unable to parse X-AMZ-Restore header: %q", s)
	}

	// len(parts) == 4

	switch parts[1] {
	case "true":
		r.OnGoingRequest = true
	case "false":
		r.OnGoingRequest = false
	default:
		return fmt.Errorf("Unable to parse X-AMZ-Restore header: %q", s)
	}

	if parts[3] != "" {
		r.ExpiryDate, err = time.Parse(time.RFC1123, parts[3])
		if err != nil {
			r.ExpiryDate, err = time.Parse(time.RFC1123Z, parts[3])
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *Bucket) GetRestoreStatus(key string) (status RestoreStatus, err error) {
	resp, err := b.Head(key, nil)
	if err != nil {
		return status, err
	}

	err = status.SetFromHeader(resp.Header)
	return status, err
}

func (r *RestoreStatus) SetFromHeader(h http.Header) (err error) {
	sc := resp.Header.Get("X-Amz-Storage-Class")

	if sc == "" {
		r.StorageClass = STANDARD
	} else {
		r.StorageClass = sc
	}

	rs := resp.Header.Get("X-Amz-Restore")

	if rs != "" {
		err = r.ParseAmzRestore(rs)
	}

	return err
}

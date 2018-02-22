package s3

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
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

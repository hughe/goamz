package s3

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"strconv"

	"github.com/hughe/goamz/s3/lifecycle"
)

func (b *Bucket) GetLifecycle() (result *lifecycle.Configuration, err error) {
	if !b.S3.v4sign {
		return nil, errors.New("SigV4 only")
	}

	params := map[string][]string{
		"lifecycle": {""},
	}

	result = &lifecycle.Configuration{}

	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			bucket: b.Name,
			params: params,
		}
		err = b.S3.query(req, result)
		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (b *Bucket) PutLifecycle(lc *lifecycle.Configuration) (err error) {
	if !b.S3.v4sign {
		return errors.New("SigV4 only")
	}

	data, err := xml.Marshal(lc)
	if err != nil {
		return err
	}

	params := map[string][]string{
		"lifecycle": {""},
	}

	digest := md5.New()
	if _, err = digest.Write(data); err != nil {
		return err
	}

	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(int64(len(data)), 10)},
		"Content-MD5":    {base64.StdEncoding.EncodeToString(digest.Sum(nil))},
		"Content-Type":   {"text/xml"},
	}

	// Can't use PutBucketSubresource because it does not provide set
	// the Content-MD5 header, or retry.

	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method:  "PUT",
			headers: headers,
			bucket:  b.Name,
			params:  params,
			payload: bytes.NewReader(data),
		}
		err = b.S3.query(req, nil)
		if ShouldRetry(err) && attempt.HasNext() {
			continue
		}
		return err
	}
	panic("unreachable")
}

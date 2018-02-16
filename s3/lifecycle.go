package s3

import "github.com/hughe/goamz/s3/lifecycle"

func (b *Bucket) GetLifecycle() (result *lifecycle.Configuration, err error) {
	// if !b.S3.v4sign {
	// 	return nil, errors.New("SigV4 only")
	// }

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

package s3

import (
	"bytes"
	"encoding/xml"
)

type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	TagSet  []Tag    `xml:"TagSet"`
}

type Tag struct {
	Key   string
	Value string
}

func (b *Bucket) PutObjectTagging(key string, tagSet []Tag) error {
	tagging := Tagging{
		TagSet: tagSet,
	}

	data, err := xml.Marshal(&tagging)
	if err != nil {
		return err
	}

	params := map[string][]string{
		"tagging": {""},
	}

	for attempt := m.Bucket.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method:  "PUT",
			bucket:  b.Name,
			path:    key,
			params:  params,
			payload: bytes.NewReader(data),
		}
		err = m.Bucket.S3.query(req, nil)
		if ShouldRetry(err) && attempt.HasNext() {
			continue
		}
		return err
	}
	panic("unreachable")
}

func (b *Bucket) GetObjectTagging(key string) (tagSet []Tag, err error) {
	params := map[string][]string{
		"tagging": {""},
	}

	result := &Tagging{}

	for attempt := m.Bucket.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			bucket: b.Name,
			path:   key,
			params: params,
		}
		err = m.Bucket.S3.query(req, result)
		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	return result.TagSet, nil
}

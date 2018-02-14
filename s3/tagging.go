package s3

import (
	"bytes"
	"encoding/xml"
	"errors"
	"sort"
)

type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	TagSet  []Tag    `xml:"TagSet>Tag"`
}

type Tag struct {
	Key   string
	Value string
}

type byKey []Tag

func (s byKey) Len() int           { return len(s) }
func (s byKey) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byKey) Less(i, j int) bool { return s[i].Key < s[j].Key }

// Set the tags on object key in bucket b.
//
// NOTE: this only works if SigV4 is enabled.  I think SigV2 signing
// code does not handle the ?tagging parameter on the URL properly.
// Not a big deal for what we use tagging for.
func (b *Bucket) PutObjectTagging(key string, tagSet map[string]string) error {
	if !b.S3.v4sign {
		return errors.New("SigV4 only")
	}

	tSet := make([]Tag, 0, len(tagSet))

	for k, v := range tagSet {
		tSet = append(tSet, Tag{Key: k, Value: v})
	}

	sort.Sort(byKey(tSet))

	t := Tagging{
		TagSet: tSet,
	}

	data, err := xml.Marshal(&t)
	if err != nil {
		return err
	}

	params := map[string][]string{
		"tagging": {""},
	}

	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method:  "PUT",
			bucket:  b.Name,
			path:    key,
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

// Get the tags assoicated with key in bucket b.

// NOTE: this only works if SigV4 is enabled.  I think SigV2 signing
// code does not handle the ?tagging parameter on the URL properly.
// Not a big deal for what we use tagging for.
func (b *Bucket) GetObjectTagging(key string) (tagSet map[string]string, err error) {
	if !b.S3.v4sign {
		return nil, errors.New("SigV4 only")
	}

	params := map[string][]string{
		"tagging": {""},
	}

	result := &Tagging{}

	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			bucket: b.Name,
			path:   key,
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

	tagSet = make(map[string]string, len(result.TagSet))

	for _, tag := range result.TagSet {
		tagSet[tag.Key] = tag.Value
	}

	return tagSet, nil
}

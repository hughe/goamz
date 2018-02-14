package s3

import (
	"bytes"
	"encoding/xml"
	"sort"
)

type Tagging struct {
	XMLName xml.Name `xml:"Tagging"`
	//XmlNs   string   `xml:"xmlns,attr"` // Ugly hack
	TagSet []Tag `xml:"TagSet>Tag"`
}

type Tag struct {
	Key   string
	Value string
}

type byKey []Tag

func (s byKey) Len() int           { return len(s) }
func (s byKey) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byKey) Less(i, j int) bool { return s[i].Key < s[j].Key }

func (b *Bucket) PutObjectTagging(key string, tagSet map[string]string) error {
	tSet := make([]Tag, 0, len(tagSet))

	for k, v := range tagSet {
		tSet = append(tSet, Tag{Key: k, Value: v})
	}

	sort.Sort(byKey(tSet))

	t := Tagging{
		TagSet: tSet,
		//XmlNs:  "http://s3.amazonaws.com/doc/2006-03-01/",
	}

	data, err := xml.Marshal(&t)
	if err != nil {
		return err
	}

	//fmt.Printf("data = %#v\n", string(data))

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

func (b *Bucket) GetObjectTagging(key string) (tagSet map[string]string, err error) {
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

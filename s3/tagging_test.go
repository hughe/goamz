package s3_test

import (
	"github.com/hughe/goamz/s3"
	. "gopkg.in/check.v1"
)

func (s *ClientTests) TestTagging(c *C) {
	if !s.isV4 {
		c.Skip("NoSigV4")
	}

	b := testBucket(s.s3)
	err := b.PutBucket(s3.PublicRead)
	c.Assert(err, IsNil)

	//fmt.Printf("b.Name = %#v\n", b.Name)

	err = b.Put("name", []byte("yo!"), "text/plain", s3.PublicRead, s3.Options{})
	c.Assert(err, IsNil)
	defer b.Del("name")

	tagSet := map[string]string{
		"tag1": "val1",
	}

	err = b.PutObjectTagging("name", tagSet)
	c.Assert(err, IsNil)

	res, err := b.GetObjectTagging("name")
	c.Assert(err, IsNil)

	c.Assert(res, DeepEquals, tagSet)

	biggerSet := map[string]string{
		"tag2": "val2",
		"tag3": "val3",
		"tag4": "val4",
	}

	err = b.PutObjectTagging("name", biggerSet)
	c.Assert(err, IsNil)

	res, err = b.GetObjectTagging("name")
	c.Assert(err, IsNil)

	c.Assert(res, DeepEquals, biggerSet)

}

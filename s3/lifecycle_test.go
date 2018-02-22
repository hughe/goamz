package s3_test

import (
	"github.com/hughe/goamz/s3"
	"github.com/hughe/goamz/s3/lifecycle"
	. "gopkg.in/check.v1"
)

// A non TLS region.
//
// func init() {
// 	USWest2NoTLS := aws.USWest2
// 	USWest2NoTLS.S3Endpoint = "http://s3-us-west-2.amazonaws.com"
// 	Suite(&AmazonClientSuite{Region: USWest2NoTLS, ClientTests: ClientTests{isV4: true}})
// }

func (s *ClientTests) TestLifecycle(c *C) {
	if !s.isV4 {
		c.Skip("NoSigV4")
	}

	b := testBucket(s.s3)
	err := b.PutBucket(s3.PublicRead)
	c.Assert(err, IsNil)

	id := "Test Lifecycle"
	prefix := "0000/S00"
	days := 2

	lc := lifecycle.Configuration{
		Rules: []lifecycle.Rule{
			{
				ID: &id,
				Filter: &lifecycle.Filter{
					Prefix: &prefix,
				},
				Status: "Enabled",
				Transitions: []lifecycle.Transition{
					{
						StorageClass: "GLACIER",
						Days:         &days,
					},
				},
			},
		},
	}

	err = b.PutLifecycle(&lc)
	c.Assert(err, IsNil)

	res, err := b.GetLifecycle()
	c.Assert(err, IsNil)

	// Res will have hte XMLName set, but lc doesn't, so zero them out
	// before comparison.
	res.XMLName.Space = ""
	res.XMLName.Local = ""

	//spew.Dump(&lc)
	//spew.Dump(res)

	c.Assert(res, DeepEquals, &lc)

}

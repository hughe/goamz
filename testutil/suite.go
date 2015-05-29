package testutil

import (
	"flag"
<<<<<<< HEAD

	"github.com/hughe/goamz/aws"
	. "gopkg.in/check.v1"
=======
	"github.com/hughe/goamz/aws"
	"github.com/motain/gocheck"
>>>>>>> e82b43a70c8a14c221b25fec952eb2795a8295fe
)

// Amazon must be used by all tested packages to determine whether to
// run functional tests against the real AWS servers.
var Amazon bool

func init() {
	flag.BoolVar(&Amazon, "amazon", false, "Enable tests against amazon server")
}

type LiveSuite struct {
	auth aws.Auth
}

func (s *LiveSuite) SetUpSuite(c *C) {
	if !Amazon {
		c.Skip("amazon tests not enabled (-amazon flag)")
	}
	auth, err := aws.EnvAuth()
	if err != nil {
		c.Fatal(err.Error())
	}
	s.auth = auth
}

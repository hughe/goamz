package s3

import (
	"net/http"
	"testing"
	"time"

	"github.com/hughe/goamz/aws"
)

func TestV4SignerBuildCanonicalRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://host.foo.com/?foo=Zoo&foo=aha", nil)
	req.Header = make(http.Header)
	req.Header.Set("Date", "Mon, 09 Sep 2011 23:36:00 GMT")
	req.Header.Set("host", "host.foo.com")

	expected := "GET\n/\nfoo=Zoo&foo=aha\ndate:Mon, 09 Sep 2011 23:36:00 GMT\nhost:host.foo.com\n\ndate;host\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	result := NewV4Signer(
		*aws.NewAuth("AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "", time.Now()),
		"host",
		aws.Regions["us-east-1"],
	).canonicalRequest(req, "")

	if result != expected {
		t.Errorf("error building canonical request, expected: \n%s, got: \n%s", expected, result)
	}
}

func TestV4SignerBuildStringToSign(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://host.foo.com/?foo=Zoo&foo=aha", nil)
	req.Header = make(http.Header)
	req.Header.Set("Date", "Mon, 09 Sep 2011 23:36:00 GMT")
	req.Header.Set("host", "host.foo.com")
	s := NewV4Signer(
		*aws.NewAuth("AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "", time.Now()),
		"host",
		aws.Regions["us-east-1"],
	)
	tm, _ := time.Parse(time.RFC3339, "2011-09-09T23:36:00Z")

	expected := "AWS4-HMAC-SHA256\n20110909T233600Z\n20110909/us-east-1/host/aws4_request\ne25f777ba161a0f1baf778a87faf057187cf5987f17953320e3ca399feb5f00d"
	result := s.stringToSign(tm, s.canonicalRequest(req, ""))

	if result != expected {
		t.Errorf("error building string to sign, expected: \n%s, got: \n%s", expected, result)
	}
}

func TestV4SignerSigns(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://host.foo.com/?foo=Zoo&foo=aha", nil)
	req.Header = make(http.Header)
	req.Header.Set("Date", "Mon, 09 Sep 2011 23:36:00 GMT")
	req.Header.Set("host", "host.foo.com")
	s := NewV4Signer(
		*aws.NewAuth("AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "", time.Now()),
		"host",
		aws.Regions["us-east-1"],
	)
	s.Sign(req)

	expected := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20110909/us-east-1/host/aws4_request, SignedHeaders=date;host;x-amz-date, Signature=8f4739ff46dfa3d0e2851c6d105247e97587a14ed1c864aa7e382b5b234ec1ca"
	result := req.Header.Get("Authorization")

	if result != expected {
		t.Errorf("error signing, expected: \n%s, got: \n%s", expected, result)
	}
}

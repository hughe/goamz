// goamz - Go packages to interact with the Amazon Web Services.
//
//   https://wiki.ubuntu.com/goamz
//
// Copyright (c) 2011 Canonical Ltd.
//
// Written by Gustavo Niemeyer <gustavo.niemeyer@canonical.com>
//

package s3

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hughe/goamz/aws"
)

var Debug = false

// The S3 type encapsulates operations with an S3 region.
type S3 struct {
	aws.Auth
	aws.Region

	// ConnectTimeout is the maximum time a request attempt will
	// wait for a successful connection to be made.
	//
	// A value of zero means no timeout.
	ConnectTimeout time.Duration

	// ReadTimeout is the maximum time a request attempt will wait
	// for an individual read to complete.
	//
	// A value of zero means no timeout.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum time a request attempt will
	// wait for an individual write to complete.
	//
	// A value of zero means no timeout.
	WriteTimeout time.Duration

	// RequestTimeout is the maximum time a request attempt can
	// take before operations return a timeout error.
	//
	// This includes connection time, any redirects, and reading
	// the response body. The timer remains running after the request
	// is made so it can interrupt reading of the response data.
	//
	// A Timeout of zero means no timeout.
	RequestTimeout time.Duration

	// AttemptStrategy is the attempt strategy used for requests.
	aws.AttemptStrategy

	// Reserve the right of using private data.
	private byte

	// client used for requests, or nil to construct our own client with retry behavior
	client *http.Client

	// set of root CA certificates is used for verifying server certificates for HTTPS connections
	// or nil to use the default system root CAs
	// only used when we construct our own client
	rootCAs *x509.CertPool

	signer *V4Signer
	v4sign bool
}

// The Bucket type encapsulates operations with an S3 bucket.
type Bucket struct {
	*S3
	Name string
}

// The Owner type represents the owner of the object in an S3 bucket.
type Owner struct {
	ID          string
	DisplayName string
}

// Fold options into an Options struct
//
type Options struct {
	SSE              bool
	Meta             map[string][]string
	ContentEncoding  string
	CacheControl     string
	RedirectLocation string
	ContentMD5       string
	// What else?
	// Content-Disposition string
	//// The following become headers so they are []strings rather than strings... I think
	// x-amz-storage-class []string
}

type CopyOptions struct {
	Options
	MetadataDirective string
	ContentType       string
}

// CopyObjectResult is the output from a Copy request
type CopyObjectResult struct {
	ETag         string
	LastModified string
}

// DefaultAttemptStrategy is the default AttemptStrategy used by S3 objects created by New.
var DefaultAttemptStrategy = aws.AttemptStrategy{
	Min:   5,
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// New creates a new S3.  Optional client argument allows for custom http.clients to be used.
func New(auth aws.Auth, region aws.Region, client ...*http.Client) *S3 {

	var httpclient *http.Client

	if len(client) > 0 {
		httpclient = client[0]
	}
	s3 := &S3{
		Auth:            auth,
		Region:          region,
		AttemptStrategy: DefaultAttemptStrategy,
		client:          httpclient,
		signer:          NewV4Signer(auth, "s3", region),
		v4sign:          false}
	s3.signer.IncludeXAmzContentSha256 = true
	return s3
}

func NewV4(auth aws.Auth, region aws.Region) *S3 {
	s3 := New(auth, region)
	s3.v4sign = true
	return s3
}

// New creates a new S3.  Optional client argument allows for custom http.clients to be used.
// rootCAs is an optional set of root CA certificates is used for verifying server certificates
// for HTTPS connections; nil means use the default system root CA set.
func NewWithCustomRootCAs(auth aws.Auth, region aws.Region, rootCAs *x509.CertPool, client ...*http.Client) *S3 {

	var httpclient *http.Client

	if len(client) > 0 {
		httpclient = client[0]
	}

	s3 := &S3{
		Auth:            auth,
		Region:          region,
		AttemptStrategy: DefaultAttemptStrategy,
		client:          httpclient,
		rootCAs:         rootCAs,
		signer:          NewV4Signer(auth, "s3", region),
		v4sign:          false}
	s3.signer.IncludeXAmzContentSha256 = true
	return s3
}

func NewWithCustomRootCAsV4(auth aws.Auth, region aws.Region, rootCAs *x509.CertPool, client ...*http.Client) *S3 {
	s3 := NewWithCustomRootCAsV4(auth, region, rootCAs, client...)
	s3.v4sign = true
	return s3
}

// Bucket returns a Bucket with the given name.
func (s3 *S3) Bucket(name string) *Bucket {
	if s3.Region.S3BucketEndpoint != "" || s3.Region.S3LowercaseBucket {
		name = strings.ToLower(name)
	}
	return &Bucket{s3, name}
}

var createBucketConfiguration = `<CreateBucketConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <LocationConstraint>%s</LocationConstraint>
</CreateBucketConfiguration>`

// locationConstraint returns an io.Reader specifying a LocationConstraint if
// required for the region.
//
// See http://goo.gl/bh9Kq for details.
func (s3 *S3) locationConstraint() io.Reader {
	constraint := ""
	if s3.Region.S3LocationConstraint {
		constraint = fmt.Sprintf(createBucketConfiguration, s3.Region.Name)
	}
	return strings.NewReader(constraint)
}

type ACL string

const (
	Private           = ACL("private")
	PublicRead        = ACL("public-read")
	PublicReadWrite   = ACL("public-read-write")
	AuthenticatedRead = ACL("authenticated-read")
	BucketOwnerRead   = ACL("bucket-owner-read")
	BucketOwnerFull   = ACL("bucket-owner-full-control")
)

// PutBucket creates a new bucket.
//
// See http://goo.gl/ndjnR for details.
func (b *Bucket) PutBucket(perm ACL) error {
	headers := map[string][]string{
		"x-amz-acl": {string(perm)},
	}
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    "/",
		headers: headers,
		payload: b.locationConstraint(),
	}
	return b.S3.query(req, nil)
}

// DelBucket removes an existing S3 bucket. All objects in the bucket must
// be removed before the bucket itself can be removed.
//
// See http://goo.gl/GoBrY for details.
func (b *Bucket) DelBucket() (err error) {
	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(); {
		req := &request{
			method: "DELETE",
			bucket: b.Name,
			path:   "/",
		}
		err = b.S3.query(req, nil)
		if !ShouldRetry(err) {
			break
		}
	}
	return err
}

// Get retrieves an object from an S3 bucket.
//
// See http://goo.gl/isCO7 for details.
func (b *Bucket) Get(path string) (data []byte, err error) {
	body, err := b.GetReader(path)
	defer func() {
		if body != nil {
			body.Close()
		}
	}()
	if err != nil {
		return nil, err
	}
	data, err = ioutil.ReadAll(body)
	return data, err
}

// GetReader retrieves an object from an S3 bucket,
// returning the body of the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading.
func (b *Bucket) GetReader(path string) (rc io.ReadCloser, err error) {
	resp, err := b.GetResponse(path)
	if resp != nil {
		return resp.Body, err
	}
	return nil, err
}

// GetResponse retrieves an object from an S3 bucket,
// returning the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading
func (b *Bucket) GetResponse(path string) (resp *http.Response, err error) {
	return b.GetResponseWithHeaders(path, make(http.Header))
}

// GetReaderWithHeaders retrieves an object from an S3 bucket
// Accepts custom headers to be sent as the second parameter
// returning the body of the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading
func (b *Bucket) GetResponseWithHeaders(path string, headers map[string][]string) (resp *http.Response, err error) {
	return b.GetResponseWithHeadersAndTimeout(path, headers, 0)
}

func (b *Bucket) GetResponseWithHeadersAndTimeout(path string, headers map[string][]string, timeout time.Duration) (resp *http.Response, err error) {
	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(); {
		req := &request{
			bucket:  b.Name,
			path:    path,
			headers: headers,
			timeout: timeout,
		}
		err = b.S3.prepare(req)
		if err != nil {
			return nil, err
		}
		resp, err := b.S3.run(req, nil)
		if ShouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	panic("unreachable")
}

// Exists checks whether or not an object exists on an S3 bucket using a HEAD request.
func (b *Bucket) Exists(path string) (exists bool, err error) {
	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(); {
		req := &request{
			method: "HEAD",
			bucket: b.Name,
			path:   path,
		}
		err = b.S3.prepare(req)
		if err != nil {
			return
		}
		resp, err := b.S3.run(req, nil)

		if ShouldRetry(err) && attempt.HasNext() {
			continue
		}

		if err != nil {
			// We can treat a 403 or 404 as non existance
			if e, ok := err.(*Error); ok && (e.StatusCode == 403 || e.StatusCode == 404) {
				return false, nil
			}
			return false, err
		}

		if resp.StatusCode/100 == 2 {
			exists = true
		}
		return exists, err
	}
	return false, fmt.Errorf("S3 Currently Unreachable")
}

// Head HEADs an object in the S3 bucket, returns the response with
// no body see http://bit.ly/17K1ylI
func (b *Bucket) Head(path string, headers map[string][]string) (*http.Response, error) {
	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(); {
		req := &request{
			method:  "HEAD",
			bucket:  b.Name,
			path:    path,
			headers: headers,
		}
		err := b.S3.prepare(req)
		if err != nil {
			return nil, err
		}
		resp, err := b.S3.run(req, nil)
		if ShouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, err
	}
	return nil, fmt.Errorf("S3 Currently Unreachable")
}

// Put inserts an object into the S3 bucket.
//
// See http://goo.gl/FEBPD for details.
func (b *Bucket) Put(path string, data []byte, contType string, perm ACL, options Options) error {
	body := bytes.NewBuffer(data)
	return b.PutReader(path, body, int64(len(data)), contType, perm, options)
}

// PutCopy puts a copy of an object given by the key path into bucket b using b.Path as the target key
func (b *Bucket) PutCopy(path string, perm ACL, options CopyOptions, source string) (*CopyObjectResult, error) {
	headers := map[string][]string{
		"x-amz-acl":         {string(perm)},
		"x-amz-copy-source": {source},
	}
	options.addHeaders(headers)
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
	}
	resp := &CopyObjectResult{}
	err := b.S3.query(req, resp)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

/*
PutHeader - like Put, inserts an object into the S3 bucket.
Instead of Content-Type string, pass in custom headers to override defaults.
*/
func (b *Bucket) PutHeader(path string, data []byte, customHeaders map[string][]string, perm ACL) error {
	body := bytes.NewBuffer(data)
	return b.PutReaderHeader(path, body, int64(len(data)), customHeaders, perm)
}

// PutReader inserts an object into the S3 bucket by consuming data
// from r until EOF.
func (b *Bucket) PutReader(path string, r io.Reader, length int64, contType string, perm ACL, options Options) error {
	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(length, 10)},
		"Content-Type":   {contType},
		"x-amz-acl":      {string(perm)},
	}
	options.addHeaders(headers)
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
		payload: r,
	}
	return b.S3.query(req, nil)
}

/*
PutReaderHeader - like PutReader, inserts an object into S3 from a reader.
Instead of Content-Type string, pass in custom headers to override defaults.
*/
func (b *Bucket) PutReaderHeader(path string, r io.Reader, length int64, customHeaders map[string][]string, perm ACL) error {
	return b.PutReaderHeaderTimeout(path, r, length, customHeaders, perm, 0)
}

func (b *Bucket) PutReaderHeaderTimeout(path string, r io.Reader, length int64, customHeaders map[string][]string, perm ACL, timeout time.Duration) error {
	// Default headers
	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(length, 10)},
		"Content-Type":   {"application/text"},
		"x-amz-acl":      {string(perm)},
	}

	// Override with custom headers
	for key, value := range customHeaders {
		headers[key] = value
	}

	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
		payload: r,
		timeout: timeout,
	}
	return b.S3.query(req, nil)
}

const sseHeaderKey = "x-amz-server-side-encryption"

// Adds a header value to the specified set of headers to request server-side encryption.
func AddHeaderSSE(headers map[string][]string) {
	headers[textproto.CanonicalMIMEHeaderKey(sseHeaderKey)] = []string{"AES256"}
}

// Returns the header value for server-side encryption, or empty string if there is no SSE header.
func GetHeaderSSE(headers map[string][]string) string {
	// Headers returned in an HTTP response will have their key names connonicalized by the Go
	// http library, so we MUST run the key through textproto.CanonicalMIMEHeaderKey in order
	// to find it in the map
	// So headers in the http request must also have been added with a cannonicalized key to find them.
	// TODO: Change parameters in this file to use http.Header data type rather than map[string][]string
	if val := headers[textproto.CanonicalMIMEHeaderKey(sseHeaderKey)]; len(val) > 0 {
		return val[0]
	}
	return ""
}

// addHeaders adds o's specified fields to headers
func (o Options) addHeaders(headers map[string][]string) {
	if o.SSE {
		AddHeaderSSE(headers)
	}
	if len(o.ContentEncoding) != 0 {
		headers["Content-Encoding"] = []string{o.ContentEncoding}
	}
	if len(o.CacheControl) != 0 {
		headers["Cache-Control"] = []string{o.CacheControl}
	}
	if len(o.ContentMD5) != 0 {
		headers["Content-MD5"] = []string{o.ContentMD5}
	}
	if len(o.RedirectLocation) != 0 {
		headers["x-amz-website-redirect-location"] = []string{o.RedirectLocation}
	}
	for k, v := range o.Meta {
		headers["x-amz-meta-"+k] = v
	}
}

// addHeaders adds o's specified fields to headers
func (o CopyOptions) addHeaders(headers map[string][]string) {
	o.Options.addHeaders(headers)
	if len(o.MetadataDirective) != 0 {
		headers["x-amz-metadata-directive"] = []string{o.MetadataDirective}
	}
	if len(o.ContentType) != 0 {
		headers["Content-Type"] = []string{o.ContentType}
	}
}

func makeXmlBuffer(doc []byte) *bytes.Buffer {
	buf := new(bytes.Buffer)
	buf.WriteString(xml.Header)
	buf.Write(doc)
	return buf
}

type RoutingRule struct {
	ConditionKeyPrefixEquals     string `xml:"Condition>KeyPrefixEquals"`
	RedirectReplaceKeyPrefixWith string `xml:"Redirect>ReplaceKeyPrefixWith,omitempty"`
	RedirectReplaceKeyWith       string `xml:"Redirect>ReplaceKeyWith,omitempty"`
}

type WebsiteConfiguration struct {
	XMLName             xml.Name       `xml:"http://s3.amazonaws.com/doc/2006-03-01/ WebsiteConfiguration"`
	IndexDocumentSuffix string         `xml:"IndexDocument>Suffix"`
	ErrorDocumentKey    string         `xml:"ErrorDocument>Key"`
	RoutingRules        *[]RoutingRule `xml:"RoutingRules>RoutingRule,omitempty"`
}

func (b *Bucket) PutBucketWebsite(configuration WebsiteConfiguration) error {

	doc, err := xml.Marshal(configuration)
	if err != nil {
		return err
	}

	buf := makeXmlBuffer(doc)

	return b.PutBucketSubresource("website", buf, int64(buf.Len()))
}

func (b *Bucket) PutBucketSubresource(subresource string, r io.Reader, length int64) error {
	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(length, 10)},
	}
	req := &request{
		path:    "/",
		method:  "PUT",
		bucket:  b.Name,
		headers: headers,
		payload: r,
		params:  url.Values{subresource: {""}},
	}

	return b.S3.query(req, nil)
}

// Del removes an object from the S3 bucket.
//
// See http://goo.gl/APeTt for details.
func (b *Bucket) Del(path string) error {
	req := &request{
		method: "DELETE",
		bucket: b.Name,
		path:   path,
	}
	return b.S3.query(req, nil)
}

type Delete struct {
	Quiet   bool     `xml:"Quiet,omitempty"`
	Objects []Object `xml:"Object"`
}

type Object struct {
	Key       string `xml:"Key"`
	VersionId string `xml:"VersionId,omitempty"`
}

// DelMulti removes up to 1000 objects from the S3 bucket.
//
// See http://goo.gl/jx6cWK for details.
func (b *Bucket) DelMulti(objects Delete) error {
	doc, err := xml.Marshal(objects)
	if err != nil {
		return err
	}

	buf := makeXmlBuffer(doc)
	digest := md5.New()
	size, err := digest.Write(buf.Bytes())
	if err != nil {
		return err
	}

	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(int64(size), 10)},
		"Content-MD5":    {base64.StdEncoding.EncodeToString(digest.Sum(nil))},
		"Content-Type":   {"text/xml"},
	}
	req := &request{
		path:    "/",
		method:  "POST",
		params:  url.Values{"delete": {""}},
		bucket:  b.Name,
		headers: headers,
		payload: buf,
	}

	return b.S3.query(req, nil)
}

// The ListResp type holds the results of a List bucket operation.
type ListResp struct {
	Name       string
	Prefix     string
	Delimiter  string
	Marker     string
	NextMarker string
	MaxKeys    int

	// IsTruncated is true if the results have been truncated because
	// there are more keys and prefixes than can fit in MaxKeys.
	// N.B. this is the opposite sense to that documented (incorrectly) in
	// http://goo.gl/YjQTc
	IsTruncated    bool
	Contents       []Key
	CommonPrefixes []string `xml:">Prefix"`
}

// The Key type represents an item stored in an S3 bucket.
type Key struct {
	Key          string
	LastModified string
	Size         int64
	// ETag gives the hex-encoded MD5 sum of the contents,
	// surrounded with double-quotes.
	ETag         string
	StorageClass string
	Owner        Owner
}

// List returns information about objects in an S3 bucket.
//
// The prefix parameter limits the response to keys that begin with the
// specified prefix.
//
// The delim parameter causes the response to group all of the keys that
// share a common prefix up to the next delimiter in a single entry within
// the CommonPrefixes field. You can use delimiters to separate a bucket
// into different groupings of keys, similar to how folders would work.
//
// The marker parameter specifies the key to start with when listing objects
// in a bucket. Amazon S3 lists objects in alphabetical order and
// will return keys alphabetically greater than the marker.
//
// The max parameter specifies how many keys + common prefixes to return in
// the response. The default is 1000.
//
// For example, given these keys in a bucket:
//
//     index.html
//     index2.html
//     photos/2006/January/sample.jpg
//     photos/2006/February/sample2.jpg
//     photos/2006/February/sample3.jpg
//     photos/2006/February/sample4.jpg
//
// Listing this bucket with delimiter set to "/" would yield the
// following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Contents:  []Key{
//             {Key: "index.html", "index2.html"},
//         },
//         CommonPrefixes: []string{
//             "photos/",
//         },
//     }
//
// Listing the same bucket with delimiter set to "/" and prefix set to
// "photos/2006/" would yield the following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Prefix:    "photos/2006/",
//         CommonPrefixes: []string{
//             "photos/2006/February/",
//             "photos/2006/January/",
//         },
//     }
//
// See http://goo.gl/YjQTc for details.
func (b *Bucket) List(prefix, delim, marker string, max int) (result *ListResp, err error) {
	params := map[string][]string{
		"prefix":    {prefix},
		"delimiter": {delim},
		"marker":    {marker},
	}
	if max != 0 {
		params["max-keys"] = []string{strconv.FormatInt(int64(max), 10)}
	}
	result = &ListResp{}
	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(); {
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

// The VersionsResp type holds the results of a list bucket Versions operation.
type VersionsResp struct {
	Name            string
	Prefix          string
	KeyMarker       string
	VersionIdMarker string
	MaxKeys         int
	Delimiter       string
	IsTruncated     bool
	Versions        []Version
	CommonPrefixes  []string `xml:">Prefix"`
}

// The Version type represents an object version stored in an S3 bucket.
type Version struct {
	Key          string
	VersionId    string
	IsLatest     bool
	LastModified string
	// ETag gives the hex-encoded MD5 sum of the contents,
	// surrounded with double-quotes.
	ETag         string
	Size         int64
	Owner        Owner
	StorageClass string
}

func (b *Bucket) Versions(prefix, delim, keyMarker string, versionIdMarker string, max int) (result *VersionsResp, err error) {
	params := map[string][]string{
		"versions":  {""},
		"prefix":    {prefix},
		"delimiter": {delim},
	}

	if len(versionIdMarker) != 0 {
		params["version-id-marker"] = []string{versionIdMarker}
	}
	if len(keyMarker) != 0 {
		params["key-marker"] = []string{keyMarker}
	}

	if max != 0 {
		params["max-keys"] = []string{strconv.FormatInt(int64(max), 10)}
	}
	result = &VersionsResp{}
	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(); {
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

// Returns a mapping of all key names in this bucket to Key objects
func (b *Bucket) GetBucketContents() (*map[string]Key, error) {
	bucket_contents := map[string]Key{}
	prefix := ""
	path_separator := ""
	marker := ""
	for {
		contents, err := b.List(prefix, path_separator, marker, 1000)
		if err != nil {
			return &bucket_contents, err
		}
		for _, key := range contents.Contents {
			bucket_contents[key.Key] = key
		}
		if contents.IsTruncated {
			marker = contents.NextMarker
		} else {
			break
		}
	}

	return &bucket_contents, nil
}

// URL returns a non-signed URL that allows retriving the
// object at path. It only works if the object is publicly
// readable (see SignedURL).
func (b *Bucket) URL(path string) string {
	req := &request{
		bucket: b.Name,
		path:   path,
	}
	err := b.S3.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url()
	if err != nil {
		panic(err)
	}
	u.RawQuery = ""
	return u.String()
}

// SignedURL returns a signed URL that allows anyone holding the URL
// to retrieve the object at path. The signature is valid until expires.
func (b *Bucket) SignedURL(path string, expires time.Time) string {
	req := &request{
		bucket: b.Name,
		path:   path,
		params: url.Values{"Expires": {strconv.FormatInt(expires.Unix(), 10)}},
	}
	err := b.S3.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url()
	if err != nil {
		panic(err)
	}
	if b.S3.Auth.Token() != "" {
		return u.String() + "&x-amz-security-token=" + url.QueryEscape(req.headers["X-Amz-Security-Token"][0])
	} else {
		return u.String()
	}
}

// UploadSignedURL returns a signed URL that allows anyone holding the URL
// to upload the object at path. The signature is valid until expires.
// contenttype is a string like image/png
// path is the resource name in s3 terminalogy like images/ali.png [obviously exclusing the bucket name itself]
func (b *Bucket) UploadSignedURL(path, method, content_type string, expires time.Time) string {
	expire_date := expires.Unix()
	if method != "POST" {
		method = "PUT"
	}
	stringToSign := method + "\n\n" + content_type + "\n" + strconv.FormatInt(expire_date, 10) + "\n/" + b.Name + "/" + path
	fmt.Println("String to sign:\n", stringToSign)
	a := b.S3.Auth
	secretKey := a.SecretKey
	accessId := a.AccessKey
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(stringToSign))
	macsum := mac.Sum(nil)
	signature := base64.StdEncoding.EncodeToString([]byte(macsum))
	signature = strings.TrimSpace(signature)

	signedurl, err := url.Parse("https://" + b.Name + ".s3.amazonaws.com/")
	if err != nil {
		log.Println("ERROR signing url for S3 upload", err)
		return ""
	}
	signedurl.Path += path
	params := url.Values{}
	params.Add("AWSAccessKeyId", accessId)
	params.Add("Expires", strconv.FormatInt(expire_date, 10))
	params.Add("Signature", signature)
	if a.Token() != "" {
		params.Add("token", a.Token())
	}

	signedurl.RawQuery = params.Encode()
	return signedurl.String()
}

// PostFormArgs returns the action and input fields needed to allow anonymous
// uploads to a bucket within the expiration limit
func (b *Bucket) PostFormArgs(path string, expires time.Time, redirect string) (action string, fields map[string]string) {
	conditions := make([]string, 0)
	fields = map[string]string{
		"AWSAccessKeyId": b.Auth.AccessKey,
		"key":            path,
	}

	conditions = append(conditions, fmt.Sprintf("{\"key\": \"%s\"}", path))
	conditions = append(conditions, fmt.Sprintf("{\"bucket\": \"%s\"}", b.Name))
	if redirect != "" {
		conditions = append(conditions, fmt.Sprintf("{\"success_action_redirect\": \"%s\"}", redirect))
		fields["success_action_redirect"] = redirect
	}

	vExpiration := expires.Format("2006-01-02T15:04:05Z")
	vConditions := strings.Join(conditions, ",")
	policy := fmt.Sprintf("{\"expiration\": \"%s\", \"conditions\": [%s]}", vExpiration, vConditions)
	policy64 := base64.StdEncoding.EncodeToString([]byte(policy))
	fields["policy"] = policy64

	signer := hmac.New(sha1.New, []byte(b.Auth.SecretKey))
	signer.Write([]byte(policy64))
	fields["signature"] = base64.StdEncoding.EncodeToString(signer.Sum(nil))

	action = fmt.Sprintf("%s/%s/", b.S3.Region.S3Endpoint, b.Name)
	return
}

type request struct {
	method   string
	bucket   string
	path     string
	signpath string
	params   url.Values
	headers  http.Header
	baseurl  string
	payload  io.Reader
	prepared bool
	timeout  time.Duration
}

func (req *request) url() (*url.URL, error) {
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return nil, fmt.Errorf("bad S3 endpoint URL %q: %v", req.baseurl, err)
	}
	u.RawQuery = req.params.Encode()
	u.Path = req.path
	return u, nil
}

// query prepares and runs the req request.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (s3 *S3) query(req *request, resp interface{}) error {
	err := s3.prepare(req)
	if err == nil {
		var httpResponse *http.Response
		httpResponse, err = s3.run(req, resp)
		if resp == nil && httpResponse != nil {
			httpResponse.Body.Close()
		}

		if err == nil {
			// If Server-Side Encryption was requested, check we got a response header
			// confirming that the data is encrypted
			if sseReqValue := GetHeaderSSE(req.headers); sseReqValue != "" {
				sseRespValue := GetHeaderSSE(httpResponse.Header)
				if sseRespValue != sseReqValue {
					// S3 didn't return matching SSE response so the requested encryption didn't happen
					return fmt.Errorf("S3 did not honor encryption request: expected x-amz-server-side-encryption response header value %q but got %q",
						sseReqValue, sseRespValue)
				}
			}
		}
	}
	return err
}

// prepare sets up req to be delivered to S3.
func (s3 *S3) prepare(req *request) error {
	var signpath = req.path

	if !req.prepared {
		req.prepared = true
		if req.method == "" {
			req.method = "GET"
		}
		// Copy so they can be mutated without affecting on retries.
		params := make(url.Values)
		headers := make(http.Header)
		for k, v := range req.params {
			params[k] = v
		}
		for k, v := range req.headers {
			headers[k] = v
		}
		req.params = params
		req.headers = headers
		if !strings.HasPrefix(req.path, "/") {
			req.path = "/" + req.path
		}
		signpath = req.path
		if req.bucket != "" {
			req.baseurl = s3.Region.S3BucketEndpoint
			if req.baseurl == "" {
				// Use the path method to address the bucket.
				req.baseurl = s3.Region.S3Endpoint
				req.path = "/" + req.bucket + req.path
			} else {
				// Just in case, prevent injection.
				if strings.IndexAny(req.bucket, "/:@") >= 0 {
					return fmt.Errorf("bad S3 bucket: %q", req.bucket)
				}
				req.baseurl = strings.Replace(req.baseurl, "${bucket}", req.bucket, -1)
			}
			signpath = "/" + req.bucket + signpath
		}
	}

	// Always sign again as it's not clear how far the
	// server has handled a previous attempt.
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return fmt.Errorf("bad S3 endpoint URL %q: %v", req.baseurl, err)
	}
	reqSignpathSpaceFix := (&url.URL{Path: signpath}).String()
	req.headers["Host"] = []string{u.Host}

	// Use GMT instead of UTC as the time zone.
	// All the examples in the S3 documentation use GMT.
	// Some S3-compatible storage providers (e.g Cloudian) require GMT
	// rather than UTC (as of June 2015)
	gmtDate := time.Now().In(time.FixedZone("GMT", 0)).Format(time.RFC1123)
	req.headers["Date"] = []string{gmtDate}

	if s3.Auth.Token() != "" {
		req.headers["X-Amz-Security-Token"] = []string{s3.Auth.Token()}
	}
	if !s3.v4sign {
		sign(s3.Auth, req.method, reqSignpathSpaceFix, req.params, req.headers)
	}
	return nil
}

// run sends req and returns the http response from the server.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (s3 *S3) run(req *request, resp interface{}) (*http.Response, error) {
	u, err := req.url()
	if err != nil {
		return nil, err
	}

	hreq := http.Request{
		URL:        u,
		Host:       u.Host,
		Method:     req.method,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Close:      true,
		Header:     req.headers,
	}

	if v, ok := req.headers["Content-Length"]; ok {
		hreq.ContentLength, _ = strconv.ParseInt(v[0], 10, 64)
		delete(req.headers, "Content-Length")
	}
	if req.payload != nil {
		hreq.Body = ioutil.NopCloser(req.payload)
	}

	if s3.v4sign {
		s3.signer.Sign(&hreq)
	}
	var httpClient *http.Client

	if s3.client != nil {
		httpClient = s3.client
	} else {
		httpClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS10,
					RootCAs:    s3.rootCAs, // nil means use the host's root CA set
				},
				Dial: func(netw, addr string) (c net.Conn, err error) {
					c, err = net.DialTimeout(netw, addr, s3.ConnectTimeout)
					if err != nil {
						return
					}

					// The timeout for this request, defaults to s3.RequestTimeout
					var thisRequestTimeout time.Duration

					// If there is a timeout set on the req, use that instead.
					if req.timeout != 0 {
						thisRequestTimeout = req.timeout
					} else {
						thisRequestTimeout = s3.RequestTimeout
					}

					var deadline time.Time
					if thisRequestTimeout > 0 {
						deadline = time.Now().Add(thisRequestTimeout)
						c.SetDeadline(deadline)

						if Debug {
							log.Printf("Set RequestTimeout: %s", thisRequestTimeout)
						}
					}

					if s3.ReadTimeout > 0 || s3.WriteTimeout > 0 {
						c = &ioTimeoutConn{
							TCPConn:         c.(*net.TCPConn),
							readTimeout:     s3.ReadTimeout,
							writeTimeout:    s3.WriteTimeout,
							requestDeadline: deadline,
						}
					}
					return
				},
				TLSHandshakeTimeout: 10 * time.Second,
			},
		}
	}

	var startTime time.Time
	if Debug {
		log.Printf("%s GoAMZ Running S3 Request: %s %s %s",
			time.Now().UTC().Format("2006/01/02 15:04:05.000"),
			hreq.Method, hreq.URL, hreq.Header)
		startTime = time.Now()
	}

	hresp, err := httpClient.Do(&hreq)
	if err != nil {
		return nil, err
	}
	if Debug {
		endTime := time.Now()
		dt := endTime.Sub(startTime)
		log.Printf("%s GoAMZ Response to S3 Request: %s %s %s %s %s %#v",
			time.Now().UTC().Format("2006/01/02 15:04:05.000"),
			hreq.Method, hreq.URL, dt, hresp.Status, hresp.Header, err)
	}
	if hresp.StatusCode != 200 && hresp.StatusCode != 204 && hresp.StatusCode != 206 {
		defer hresp.Body.Close()
		return nil, buildError(hresp)
	}
	if resp != nil {
		err = xml.NewDecoder(hresp.Body).Decode(resp)
		hresp.Body.Close()
		if Debug {
			log.Printf("goamz.s3> decoded xml into %#v", resp)
		}
	}
	return hresp, err
}

// Error represents an error in an operation with S3.
type Error struct {
	StatusCode      int    // HTTP status code (200, 403, ...)
	Code            string // EC2 error code ("UnsupportedOperation", ...)
	Message         string // The human-oriented error message
	BucketName      string
	RequestId       string
	HostId          string
	ResponseHeaders http.Header
}

func (e *Error) Error() string {
	return e.Message
}

func buildError(r *http.Response) error {
	if Debug {
		log.Printf("got error (status code %v)", r.StatusCode)
	}

	data, readErr := ioutil.ReadAll(r.Body)
	r.Body.Close()

	if Debug {
		if readErr != nil {
			log.Printf("\tread error: %v", readErr)
		} else {
			log.Printf("\tdata:\n%s\n\n", data)
		}
	}

	err := Error{ResponseHeaders: r.Header}
	errMessage := ""
	if readErr != nil {
		errMessage = fmt.Sprintf("reading response body failed: %s\n", readErr.Error())
	} else {
		decodeErr := xml.Unmarshal(data, &err)
		if decodeErr != nil {
			errMessage = fmt.Sprintf("decoding XML failed: %s\n", decodeErr.Error())
		}
	}
	// could be empty due to read or decode error or s3 didn't return an error message in XML body
	if err.Message == "" {
		if readErr == nil {
			errMessage = fmt.Sprintf("%sraw response: \n%s\n", errMessage, data)
		}
		err.Message = fmt.Sprintf("HTTP response '%s': %s", r.Status, errMessage)
	}
	err.StatusCode = r.StatusCode

	if Debug {
		log.Printf("err: %#v\n", err)
	}

	return &err
}

var RetryAlmostAllErrors bool = true

func ShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if Debug {
		log.Printf("got error: %s (%#v)", err.Error(), err)
	}
	if RetryAlmostAllErrors {
		return shouldRetryAlmostAll(err)
	}
	return shouldRetrySpecific(err)
}

func shouldRetrySpecific(err error) bool {
	// This is the body of the orginal ShouldRetry function.  It has
	// lots of different cases for different kinds of error that come
	// from the HTTP library, the network library, S3 and the Moon.
	// We tried to make this exhaustive, but that seems impossible.

	if e, ok := err.(*url.Error); ok {
		// Transport returns this string if it detects a write on a connection which
		// has already had an error
		if e.Err.Error() == "http: can't write HTTP request on broken connection" {
			return true
		}
		err = e.Err
	}

	// Errors from the http and net packages can implement a Timeout
	// method.  The following checks err to see if it has a Timeout
	// method and checks it to see if we should retry.
	type timeoutInterface interface {
		Timeout() bool
	}

	if tErr, ok := err.(timeoutInterface); ok && tErr.Timeout() {
		log.Println("Request timeout: ", err.Error())
		return true
	}

	// Same thing but for the Temporary method.  We could combine this
	// with timeoutInterface, but this way is a little more flexible.
	type temporaryInterface interface {
		Temporary() bool
	}

	if tErr, ok := err.(temporaryInterface); ok && tErr.Temporary() {
		log.Println("Temporary error: ", err.Error())
		return true
	}

	switch err.Error() {
	// This one does not have a type or a variable, which is bad.  It
	// happens if an http.Client timesout while connecting.
	case "net/http: request canceled while waiting for connection":
		return true

		// Ditto
	case "use of closed network connection":
		return true
	}

	switch err {
	case io.ErrUnexpectedEOF, io.EOF:
		return true
	}
	switch e := err.(type) {
	case *net.DNSError:
		return true
	case *net.OpError:
		switch e.Op {
		case "read", "write", "WSARecv", "WSASend", "ConnectEx":
			return true
		}
	case *Error:
		switch e.Code {
		case "InternalError", "NoSuchUpload", "NoSuchBucket", "RequestTimeout":
			return true
		}

		// 500-series responses should always be retried
		if e.StatusCode >= 500 && e.StatusCode <= 599 {
			return true
		}

		switch e.StatusCode {
		case 429: // http.StatusTooManyRequests:
			// Rate limiting I think ...
			return true
		case http.StatusConflict: // 409
			return shouldRetryConflictError(e)
		default:
			// Fallthrough
		}

		// Sometimes we get a 400 error with no Code, Message, RequestID, ...
		// I don't think it even comes from S3, maybe a load balancer or something.
		// Anyway whatever causes it, it's not a bad request.
		if e.StatusCode == http.StatusBadRequest && e.Code == "" {
			return true
		}
	}
	return false
}

func shouldRetryAlmostAll(err error) bool {
	// Strip aways the url.Error wrapper if we got one.
	if e, ok := err.(*url.Error); ok {
		err = e.Err
	}

	if e, ok := err.(*Error); ok {
		// Error came from S3 (or some proxy)
		if e.StatusCode < 500 {
			// Codes < 500 should not be retried except ...

			switch e.Code {
			case "InternalError", "NoSuchUpload", "NoSuchBucket", "RequestTimeout":
				return true
			}

			switch e.StatusCode {
			case 429: // http.StatusTooManyRequests:
				// Rate limiting I think ...
				return true
			case http.StatusConflict: // 409
				return shouldRetryConflictError(e)
			default:
				// Fallthrough
			}

			// ... Sometimes we get a 400 error with no Code, Message, RequestID, ...
			// I don't think it even comes from S3, maybe a load balancer or something.
			// Anyway whatever causes it, it's not a bad request.
			if e.StatusCode == http.StatusBadRequest && e.Code == "" {
				return true
			}

			return false
		}
	}

	// We should retry everything, unless we have decided not to
	// above.
	return true
}

func shouldRetryConflictError(err *Error) bool {
	if err.StatusCode != http.StatusConflict {
		panic("Expected error to have HTTP 409 status code")
	}
	// HDS HCP can return a conflicting operation error when two HTTP requests
	// simultaneously PUT to the same key. This is most often observed when
	// the first HTTP request has timed out, and a second retry request runs before
	// HCP has recognized the first attempt is now terminated.
	//
	// Additionally, on HCP, this must be differentiated from a 409 error where
	// an object already exists, which should never be retried. The only
	// way to identify this is by checking the response body text.
	// This differentiation is not needed on other platforms (e.g. S3) as they
	// have last-write wins semantics, making PUTs idempotent, and will not
	// return an object already exists error.
	server := err.ResponseHeaders.Get("Server")
	// retry on all platforms except HCP
	if !strings.HasPrefix(server, "HCP") {
		return true
	}
	// only retry on HCP if this is a conflicting operation error NOT an object already exists error
	if err.Code == "OperationAborted" && strings.Contains(strings.ToLower(err.Error()), "conflicting operation") {
		return true
	}
	return false
}

func hasCode(err error, code string) bool {
	s3err, ok := err.(*Error)
	return ok && s3err.Code == code
}

// ioTimeoutConn is a net.Conn which sets a deadline for each Read or Write operation
type ioTimeoutConn struct {
	*net.TCPConn
	readTimeout     time.Duration
	writeTimeout    time.Duration
	requestDeadline time.Time
}

func (c *ioTimeoutConn) deadline(timeout time.Duration) time.Time {
	dl := time.Now().Add(timeout)
	if c.requestDeadline.IsZero() || dl.Before(c.requestDeadline) {
		return dl
	}

	return c.requestDeadline
}

func (c *ioTimeoutConn) Read(b []byte) (int, error) {
	if c.readTimeout > 0 {
		err := c.TCPConn.SetReadDeadline(c.deadline(c.readTimeout))
		if err != nil {
			return 0, err
		}
	}
	return c.TCPConn.Read(b)
}

func (c *ioTimeoutConn) Write(b []byte) (int, error) {
	if c.writeTimeout > 0 {
		err := c.TCPConn.SetWriteDeadline(c.deadline(c.writeTimeout))
		if err != nil {
			return 0, err
		}
	}
	return c.TCPConn.Write(b)
}

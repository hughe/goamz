To run all the tests, without hitting AWS:

    go test
	
To hit AWS (see the list of regions in `s3i_test.go`):

    go test -amazon
	
you must set the `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
environment vars first.

For more verbosity:

    go test -amazon -check.v 
	
To run a subset of the tests:
	
    go test -amazon -check.v -check.f=REGEX
	

	


package s3

type ListClonesInProgressResult struct {
	ClonesInProgress []CloneInProgress // array of UUIDs
}
type CloneInProgress struct {
	CloneId    string
	SrcBucket  string
	DestBucket string
}

// Note: This will list all in-progress clone jobs across ALL buckets
func (s3 *S3) ListClones() (clones []CloneInProgress, err error) {
	params := map[string][]string{
		"x-storreduce-clones-in-progress": []string{""},
	}
	var resp ListClonesInProgressResult
	for attempt := s3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method: "GET",
			params: params,
		}
		req.rootCloneOp = true
		err = s3.query(req, &resp)

		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return resp.ClonesInProgress, nil
}

type CloneBucketResp struct {
	CloneId string
}

func (b *Bucket) StartClone(srcBucket string) (cloneBucketResp *CloneBucketResp, err error) {
	params := map[string][]string{
		"x-storreduce-clone": []string{srcBucket},
	}
	for attempt := b.S3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method: "PUT",
			params: params,
			bucket: b.Name,
		}
		req.rootCloneOp = true
		cloneBucketResp = &CloneBucketResp{}
		err = b.S3.query(req, cloneBucketResp)

		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return cloneBucketResp, nil
}

func (s3 *S3) CompleteClone(cloneId string) (err error) {
	params := map[string][]string{
		"x-storreduce-complete-clone": []string{cloneId},
	}
	for attempt := s3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method: "PUT",
			params: params,
		}
		req.rootCloneOp = true

		err = s3.query(req, nil)

		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func (s3 *S3) AbortClone(cloneId string) (err error) {
	params := map[string][]string{
		"x-storreduce-abort-clone": []string{cloneId},
	}
	for attempt := s3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method: "DELETE",
			params: params,
		}
		req.rootCloneOp = true

		err = s3.query(req, nil)

		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func (s3 *S3) CompleteAbortClone(cloneId string) (err error) {
	params := map[string][]string{
		"x-storreduce-complete-abort-clone": []string{cloneId},
	}
	for attempt := s3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method: "DELETE",
			params: params,
		}
		req.rootCloneOp = true
		err = s3.query(req, nil)

		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return err
	}
	return nil
}

type StatusReport struct {
	InternalStateOr__  int32
	InternalStateAnd__ int32
	State              string
	Description        string
	SrcBucket          string `xml:",omitempty"`
	DestBucket         string `xml:",omitempty"`
	KeysCloned         int64  `xml:",omitempty"`
	KeysReverted       int64  `xml:",omitempty"`
}

func (s3 *S3) GetCloneStatus(cloneId string) (resp *StatusReport, err error) {
	params := map[string][]string{
		"x-storreduce-clone-status": []string{cloneId},
	}
	for attempt := s3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method: "GET",
			params: params,
		}
		resp = &StatusReport{}
		req.rootCloneOp = true

		err = s3.query(req, resp)

		if !ShouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return resp, nil
}

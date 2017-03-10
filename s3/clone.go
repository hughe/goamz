package s3

type ListClonesInProgressResult struct {
	ClonesInProgress []string // array of UUIDs
}

// Note: This will list all in-progress clone jobs across ALL buckets
func (s3 *S3) ListClones() (clones []string, err error) {
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

type statusReport struct {
	//DebugSampleCloneBucketState database.CloneBucketState
	RawStateOr  int32
	RawStateAnd int32
	State       string
	Message     string
}

func (s3 *S3) GetCloneStatus(cloneId string) (resp *statusReport, err error) {
	params := map[string][]string{
		"x-storreduce-clone-status": []string{cloneId},
	}
	for attempt := s3.AttemptStrategy.Start(); attempt.Next(err); {
		req := &request{
			method: "GET",
			params: params,
		}
		resp = &statusReport{}
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

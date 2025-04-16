package work

import (
	"sync"
	"time"

	"github.com/grafana/tempo/pkg/tempopb"
	"github.com/grafana/tempo/pkg/util"
)

type Job struct {
	ID          string            `json:"id"`
	Type        tempopb.JobType   `json:"type"`
	JobDetail   tempopb.JobDetail `json:"job_detail"`
	Status      tempopb.JobStatus `json:"status"`
	mtx         sync.RWMutex
	CreatedTime time.Time `json:"created_time"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	WorkerID    string    `json:"worker_id"`
	Retries     int       `json:"retries"`
}

func (j *Job) Start() {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.Status = tempopb.JobStatus_JOB_STATUS_RUNNING
	j.StartTime = time.Now()
}

func (j *Job) Complete() {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.Status = tempopb.JobStatus_JOB_STATUS_SUCCEEDED
	j.EndTime = time.Now()
}

func (j *Job) Fail() {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	j.Status = tempopb.JobStatus_JOB_STATUS_FAILED
	j.EndTime = time.Now()
}

func (j *Job) IsComplete() bool {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.Status == tempopb.JobStatus_JOB_STATUS_SUCCEEDED
}

func (j *Job) IsFailed() bool {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.Status == tempopb.JobStatus_JOB_STATUS_FAILED
}

func (j *Job) IsPending() bool {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.Status == tempopb.JobStatus_JOB_STATUS_UNSPECIFIED
}

func (j *Job) IsRunning() bool {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.Status == tempopb.JobStatus_JOB_STATUS_RUNNING
}

func (j *Job) GetID() string {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.ID
}

func (j *Job) GetStatus() tempopb.JobStatus {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.Status
}

func (j *Job) GetCreatedTime() time.Time {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.CreatedTime
}

func (j *Job) GetStartTime() time.Time {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.StartTime
}

func (j *Job) GetEndTime() time.Time {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.EndTime
}

func (j *Job) GetType() tempopb.JobType {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.Type
}

func (j *Job) SetWorkerID(id string) {
	j.mtx.Lock()
	defer j.mtx.Unlock()
	j.WorkerID = id
}

func (j *Job) GetWorkerID() string {
	j.mtx.RLock()
	defer j.mtx.RUnlock()
	return j.WorkerID
}

// OnBlock returns true if the job is operating on a block.
func (j *Job) OnBlock(id string) bool {
	for _, b := range j.GetCompactionInput() {
		if b == id {
			return true
		}
	}

	// TODO: is this required?  Perhaps we only need to skip the inputs.  A
	// redaction job will possibly use the output of a compaction job.
	for _, b := range j.GetCompactionOutput() {
		if b == id {
			return true
		}
	}

	for _, b := range j.GetRedactionBlocks() {
		if b == id {
			return true
		}
	}

	return false
}

// OnTrace returns true if the job is operating on a TraceID.
func (j *Job) OnTraceID(id string) bool {
	for _, b := range j.GetRedactionTraceIDs() {
		if b == id {
			return true
		}
	}

	return false
}

func (j *Job) Tenant() string {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	return j.JobDetail.Tenant
}

func (j *Job) GetCompactionInput() []string {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	switch j.Type {
	case tempopb.JobType_JOB_TYPE_COMPACTION:
		return j.JobDetail.Compaction.Input
	default:
		return nil
	}
}

func (j *Job) GetCompactionOutput() []string {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	switch j.Type {
	case tempopb.JobType_JOB_TYPE_COMPACTION:
		return j.JobDetail.Compaction.Output
	default:
		return nil
	}
}

func (j *Job) SetCompactionOutput(blocks []string) {
	j.mtx.Lock()
	defer j.mtx.Unlock()

	switch j.Type {
	case tempopb.JobType_JOB_TYPE_COMPACTION:
		j.JobDetail.Compaction.Output = blocks
	default:
		return
	}
}

func (j *Job) GetRedactionTraceIDs() []string {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	switch j.Type {
	case tempopb.JobType_JOB_TYPE_REDACTION:
		if len(j.JobDetail.Redaction.TraceIDs) == 0 {
			return nil
		}

		ids := make([]string, len(j.JobDetail.Redaction.TraceIDs))

		for i, id := range j.JobDetail.Redaction.TraceIDs {
			ids[i] = util.TraceIDToHexString(id)
		}

		return ids
	default:
		return nil
	}
}

func (j *Job) GetRedactionBlocks() []string {
	j.mtx.RLock()
	defer j.mtx.RUnlock()

	switch j.Type {
	case tempopb.JobType_JOB_TYPE_REDACTION:
		return j.JobDetail.Redaction.Blocks
	default:
		return nil
	}
}

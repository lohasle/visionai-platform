package visionai

import "fmt"

type JobStatus string

const (
	JobPending   JobStatus = "PENDING"
	JobQueued    JobStatus = "QUEUED"
	JobRunning   JobStatus = "RUNNING"
	JobSucceeded JobStatus = "SUCCEEDED"
	JobFailed    JobStatus = "FAILED"
	JobCancelled JobStatus = "CANCELLED"
)

var jobTransitions = map[JobStatus]map[JobStatus]struct{}{
	JobPending: {JobQueued: {}, JobCancelled: {}},
	JobQueued:  {JobRunning: {}, JobFailed: {}, JobCancelled: {}},
	JobRunning: {JobSucceeded: {}, JobFailed: {}, JobCancelled: {}},
	JobFailed:  {JobQueued: {}},
}

func CanTransitionJob(from, to JobStatus) bool {
	allowed, ok := jobTransitions[from]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}

func TransitionJob(job *PlatformJob, to JobStatus) error {
	if !CanTransitionJob(job.Status, to) {
		return fmt.Errorf("STATE_JOB_TRANSITION_NOT_ALLOWED: %s -> %s", job.Status, to)
	}
	job.Status = to
	switch to {
	case JobSucceeded:
		job.Progress = 100
		job.ErrorCode, job.ErrorMessage, job.Remediation = "", "", ""
	case JobQueued:
		job.CancelAt, job.FinishedAt = nil, nil
	}
	return nil
}

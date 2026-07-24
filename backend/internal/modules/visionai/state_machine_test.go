package visionai

import "testing"

func TestPlatformJobStateMachine(t *testing.T) {
	tests := []struct {
		from, to JobStatus
		want     bool
	}{
		{JobPending, JobQueued, true},
		{JobQueued, JobRunning, true},
		{JobRunning, JobSucceeded, true},
		{JobRunning, JobFailed, true},
		{JobFailed, JobQueued, true},
		{JobSucceeded, JobRunning, false},
		{JobCancelled, JobQueued, false},
		{JobPending, JobSucceeded, false},
	}
	for _, test := range tests {
		if got := CanTransitionJob(test.from, test.to); got != test.want {
			t.Errorf("CanTransitionJob(%s, %s) = %v, want %v", test.from, test.to, got, test.want)
		}
	}
}

func TestSuccessfulJobCompletesProgress(t *testing.T) {
	job := PlatformJob{Status: JobRunning, Progress: 72, ErrorCode: "RUNTIME_TEST"}
	if err := TransitionJob(&job, JobSucceeded); err != nil {
		t.Fatal(err)
	}
	if job.Progress != 100 || job.ErrorCode != "" {
		t.Fatalf("unexpected completed job: %#v", job)
	}
}

package jobs

type JobStatus string

const (
	JobRunning     JobStatus = "running"
	JobInterrupted JobStatus = "interrupted"
	JobFailed      JobStatus = "failed"
	JobEnded       JobStatus = "ended"
	JobCompleted   JobStatus = "completed"
)

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepSuccess StepStatus = "success"
	StepFailed  StepStatus = "failed"
	StepSkipped StepStatus = "skipped"
)

type StepType string

const (
	StepAdunitList StepType = "adunit_list"
	StepSummary    StepType = "summary"
	StepDetail     StepType = "detail"
	StepSettlement StepType = "settlement"
)

var OrderedSteps = []StepType{
	StepAdunitList,
	StepSummary,
	StepDetail,
	StepSettlement,
}

func CanResume(status JobStatus) bool {
	return status == JobInterrupted || status == JobFailed
}

func IsTerminal(status JobStatus) bool {
	return status == JobEnded || status == JobCompleted
}

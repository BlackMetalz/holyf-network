package shared

type Diagnosis struct {
	Severity   HealthLevel
	Confidence string
	Issue      string
	Scope      string
	Signal     string
	Likely     string
	Check      string
	Headline   string
	Reason     string
	Evidence   []string
	NextChecks []string
}

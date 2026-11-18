package analysis

// Classification describes how a derived metric relates to its evidence.
type Classification string

const (
	VerifiedFromSource      Classification = "verified_from_source"
	DerivedExact            Classification = "derived_exact"
	DerivedUnderAssumptions Classification = "derived_under_assumptions"
	StatisticalEstimate     Classification = "statistical_estimate"
	Modeled                 Classification = "modeled"
	Unknown                 Classification = "unknown"
	Unavailable             Classification = "unavailable"
	ConflictingSources      Classification = "conflicting_sources"
)

// Completeness describes coverage of inputs for a report.
type Completeness string

const (
	Complete             Completeness = "complete"
	Partial              Completeness = "partial"
	Sampled              Completeness = "sampled"
	BoundedDepth         Completeness = "bounded_ranking_depth"
	IncompletePagination Completeness = "incomplete_pagination"
	NotComparable        Completeness = "not_comparable"
	UnknownCompleteness  Completeness = "unknown"
)

// Provenance attaches evidence metadata to a derived metric.
type Provenance struct {
	MetricID        string            `json:"metricId"`
	MethodID        string            `json:"methodId"`
	Classification  Classification    `json:"classification"`
	Completeness    Completeness      `json:"completeness"`
	SourceView      string            `json:"sourceView,omitempty"`
	SourceFieldPath string            `json:"sourceFieldPath,omitempty"`
	SourceManifest  string            `json:"sourceManifest,omitempty"`
	SourceTimestamp string            `json:"sourceTimestamp,omitempty"`
	RawValues       map[string]string `json:"rawValues,omitempty"`
	Transformation  string            `json:"transformation,omitempty"`
	Result          string            `json:"result,omitempty"`
	Units           string            `json:"units,omitempty"`
	Rounded         bool              `json:"rounded,omitempty"`
	Assumptions     []string          `json:"assumptions,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
	Conflicts       []string          `json:"conflicts,omitempty"`
}

// CheckStatus is one reconciliation/audit check outcome.
type CheckStatus string

const (
	CheckPass         CheckStatus = "pass"
	CheckFail         CheckStatus = "fail"
	CheckInconclusive CheckStatus = "inconclusive"
	CheckSkipped      CheckStatus = "skipped"
)

// CheckResult is one named integrity or reconciliation check.
type CheckResult struct {
	ID       string      `json:"id"`
	Status   CheckStatus `json:"status"`
	Detail   string      `json:"detail,omitempty"`
	Evidence []string    `json:"evidence,omitempty"`
}

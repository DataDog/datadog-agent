/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
package proxy

type Source string

const (
	SourceDefault Source = "default"
	SourceStdEnv  Source = "std_env"
	SourceConfig  Source = "config"
	SourceDDEnv   Source = "dd_env"
)

type ValueWithSource struct {
	Value  string `json:"value"`
	Source Source `json:"source"`
}

type Effective struct {
	HTTP            ValueWithSource `json:"http"`
	HTTPS           ValueWithSource `json:"https"`
	NoProxy         ValueWithSource `json:"no_proxy"`
	NonExactNoProxy bool            `json:"non_exact_no_proxy"`
}

type Severity string

const (
	SeverityGreen  Severity = "green"
	SeverityYellow Severity = "yellow"
	SeverityRed    Severity = "red"
)

type Finding struct {
	Code        string   `json:"code"`
	Severity    Severity `json:"severity"`
	Description string   `json:"description"`
	Action      string   `json:"action"`
	Evidence    any      `json:"evidence,omitempty"`
}

type Endpoint struct {
	Name string `json:"name"` // control, logs, apm, process, rc, etc.
	URL  string `json:"url"`
}

type EndpointCheck struct {
	Endpoint Endpoint `json:"endpoint"`
	Host     string   `json:"host"`
	Port     string   `json:"port"`
	Bypassed bool     `json:"bypassed"`
	Matched  string   `json:"matched_token,omitempty"`
}

type Conflict struct {
	Key    string            `json:"key"`
	Values []ValueWithSource `json:"values"`
}

type Result struct {
	Summary        Severity        `json:"summary"`
	Effective      Effective       `json:"effective"`
	Findings       []Finding       `json:"findings"`
	EndpointMatrix []EndpointCheck `json:"endpoint_matrix"` // always present (possibly [])
	Conflicts      []Conflict      `json:"conflicts,omitempty"`
}

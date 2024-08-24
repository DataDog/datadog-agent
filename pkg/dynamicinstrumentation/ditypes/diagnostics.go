// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ditypes

// DiagnosticUpload is the message sent to the DataDog backend conveying diagnostic information
type DiagnosticUpload struct {
	Service  string `json:"service"`
	DDSource string `json:"ddsource"`

	Debugger struct {
		Diagnostic `json:"diagnostics"`
	} `json:"debugger"`
}

// SetError sets the error in the diagnostic upload
func (d *DiagnosticUpload) SetError(errorType, errorMessage string) {
	d.Debugger.Diagnostic.Status = StatusError
	d.Debugger.Diagnostic.DiagnosticException = &DiagnosticException{
		Type:    errorType,
		Message: errorMessage,
	}
}

// Status conveys the status of a probe
type Status string

const (
	StatusReceived  Status = "RECEIVED"  // StatusReceived means the probe configuration was received
	StatusInstalled Status = "INSTALLED" // StatusInstalled means the probe was installed
	StatusEmitting  Status = "EMITTING"  // StatusEmitting means the probe is emitting events
	StatusError     Status = "ERROR"     // StatusError means the probe has an issue
)

// Diagnostic contains fields relevant for conveying the status of a probe
type Diagnostic struct {
	RuntimeID string `json:"runtimeId"`
	ProbeID   string `json:"probeId"`
	Status    Status `json:"status"`

	*DiagnosticException `json:"exception,omitempty"`
}

// DiagnosticException is used for diagnosing errors in probes
type DiagnosticException struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

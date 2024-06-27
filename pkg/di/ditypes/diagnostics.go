package ditypes

type DiagnosticUpload struct {
	Service  string `json:"service"`
	DDSource string `json:"ddsource"`

	Debugger struct {
		Diagnostic `json:"diagnostics"`
	} `json:"debugger"`
}

func (d *DiagnosticUpload) SetError(errorType, errorMessage string) {
	d.Debugger.Diagnostic.Status = StatusError
	d.Debugger.Diagnostic.DiagnosticException = &DiagnosticException{
		Type:    errorType,
		Message: errorMessage,
	}
}

type Status string

const (
	StatusReceived  Status = "RECEIVED"
	StatusInstalled Status = "INSTALLED"
	StatusEmitting  Status = "EMITTING"
	StatusError     Status = "ERROR"
)

type Diagnostic struct {
	RuntimeID string `json:"runtimeId"`
	ProbeID   string `json:"probeId"`
	Status    Status `json:"status"`

	*DiagnosticException `json:"exception,omitempty"`
}

type DiagnosticException struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

package secrets

// SecretVal defines the structure for secrets in JSON output
type SecretVal struct {
	Value    string `json:"value,omitempty"`
	ErrorMsg string `json:"error,omitempty"`
}

// PayloadVersion defines the current payload version sent to a secret backend
const PayloadVersion = "1.0"

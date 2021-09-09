package backend

import "github.com/rapdev-io/datadog-secret-backend/secret"

type ErrorBackend struct {
	BackendId string
	Error     error
}

func NewErrorBackend(backendId string, e error) *ErrorBackend {
	return &ErrorBackend{BackendId: backendId, Error: e}
}

func (b *ErrorBackend) GetSecretOutput(secretKey string) secret.SecretOutput {
	es := b.Error.Error()
	return secret.SecretOutput{
		Value: nil,
		Error: &es,
	}
}

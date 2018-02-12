package processor

import (
	"fmt"
)

// Prefixer adds a prefix to a message.
type Prefixer interface {
	prefix(content []byte) []byte
}

// framePrefixer is responsible for prefixing frames being sent with the API key.
type apiKeyPrefixer struct {
	Prefixer
	key []byte
}

// NewAPIKeyPrefixer returns a prefixer that prepends the given API key to a message.
func NewAPIKeyPrefixer(apikey, logset string) Prefixer {
	if logset != "" {
		apikey = fmt.Sprintf("%s/%s", apikey, logset)
	}
	return &apiKeyPrefixer{
		key: append([]byte(apikey), ' '),
	}
}

func (p *apiKeyPrefixer) prefix(content []byte) []byte {
	return append(p.key, content...)
}

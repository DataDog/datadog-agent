package common

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var token string

// SetSessionToken
func SetSessionToken() error {
	if token != "" {
		return fmt.Errorf("session token already set")
	}

	token = config.Datadog.GetString("api_key") //encode this into JWT
	return nil
}

// GetSessionToken
func GetSessionToken() string {
	// FIXME: make this a real session id
	return token
}

func Validate(r *http.Request) error {
	tok := r.Header.Get("Session-Token")
	if tok == "" {
		return fmt.Errorf("no session token available")
	}

	if tok != GetSessionToken() {
		return fmt.Errorf("invalid session token")
	}

	return nil
}

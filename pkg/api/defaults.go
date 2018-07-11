package api

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/api/util"
)

// LocalhostHosts is the hostname list for localhost
var LocalhostHosts = []string{"127.0.0.1", "localhost"}

// DefaultTokenValidator uses the default auth_token to validate requests
func DefaultTokenValidator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}

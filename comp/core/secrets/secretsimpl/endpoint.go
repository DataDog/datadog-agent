package secretsimpl

import (
	"encoding/json"
	"net/http"
)

type InfoProvider struct {
	secretResolver *secretResolver
}

type RefreshProvider struct {
	secretResolver *secretResolver
}

func (p InfoProvider) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	p.secretResolver.GetDebugInfo(w)
}

func (p RefreshProvider) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	result, err := p.secretResolver.Refresh()
	if err != nil {
		setJSONError(w, err, 500)
		return
	}
	w.Write([]byte(result))
}

// TODO: move this to a api util function
func setJSONError(w http.ResponseWriter, err error, errorCode int) {
	w.Header().Set("Content-Type", "application/json")
	body, _ := json.Marshal(map[string]string{"error": err.Error()})
	http.Error(w, string(body), errorCode)
}

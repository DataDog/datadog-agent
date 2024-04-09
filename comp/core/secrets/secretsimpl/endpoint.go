package secretsimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/utils"
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
		utils.SetJSONError(w, err, 500)
		return
	}
	w.Write([]byte(result))
}

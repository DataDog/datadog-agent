package workloadmeta

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/utils"
)

type EndpointProvider struct {
	wmeta *workloadmeta
}

func (e EndpointProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	wmeta := e.wmeta
	verbose := false
	params := r.URL.Query()
	if v, ok := params["verbose"]; ok {
		if len(v) >= 1 && v[0] == "true" {
			verbose = true
		}
	}

	response := wmeta.Dump(verbose)
	jsonDump, err := json.Marshal(response)
	if err != nil {
		utils.SetJSONError(w, wmeta.log.Errorf("Unable to marshal workload list response: %v", err), 500)
		return
	}

	w.Write(jsonDump)
}

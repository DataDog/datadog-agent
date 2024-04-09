package autodiscoveryimpl

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/response"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type EndpointProvider struct {
	ac *AutoConfig
}

func (e EndpointProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var response response.ConfigCheckResponse

	configSlice := e.ac.LoadedConfigs()
	sort.Slice(configSlice, func(i, j int) bool {
		return configSlice[i].Name < configSlice[j].Name
	})
	response.Configs = configSlice
	response.ResolveWarnings = GetResolveWarnings()
	response.ConfigErrors = GetConfigErrors()
	response.Unresolved = e.ac.GetUnresolvedTemplates()

	jsonConfig, err := json.Marshal(response)
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Unable to marshal config check response: %s", err), 500)
		return
	}

	w.Write(jsonConfig)

}

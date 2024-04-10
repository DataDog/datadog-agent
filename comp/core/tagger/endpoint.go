package tagger

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/utils"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type EndpointProvider struct {
	tagger *TaggerClient
}

func (e *EndpointProvider) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	// query at the highest cardinality between checks and dogstatsd cardinalities
	cardinality := collectors.TagCardinality(max(int(ChecksCardinality), int(DogstatsdCardinality)))
	response := e.tagger.List(cardinality)

	jsonTags, err := json.Marshal(response)
	if err != nil {
		utils.SetJSONError(w, log.Errorf("Unable to marshal tagger list response: %s", err), 500)
		return
	}
	w.Write(jsonTags)
}

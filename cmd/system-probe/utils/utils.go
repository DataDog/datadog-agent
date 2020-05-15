package utils

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// WriteAsJSON marshals the give data argument into JSON and writes it to the `http.ResponseWriter`
func WriteAsJSON(w http.ResponseWriter, data interface{}) {
	buf, err := json.Marshal(data)
	if err != nil {
		log.Errorf("unable to marshall connections into JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Write(buf) //nolint:errcheck
}

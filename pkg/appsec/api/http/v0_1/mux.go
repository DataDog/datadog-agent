package v0_1

import (
	"encoding/json"
	"net/http"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
	"github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1/types"
)

func NewServeMux(c agenttypes.TraceChan) *http.ServeMux {
	mux := http.NewServeMux()

	s := server{c}
	mux.HandleFunc("/traces", s.HandleTraces)
	return mux
}

type server struct {
	c agenttypes.TraceChan
}

func (s *server) HandleTraces(w http.ResponseWriter, r *http.Request) {
	switch ct := r.Header.Get("Content-Type"); ct {
	case "json":
		handleJSONTraces(w, r, c)
	default:
		http.Error(w, "unexpected Content-Type value", http.StatusBadRequest)
	}
}

func handleJSONTraces(w http.ResponseWriter, r *http.Request, c agenttypes.TraceChan) {
	var traces types.RawJSONTraceSlice
	if err := json.NewDecoder(r.Body).Decode(&traces); err != nil {
		http.Error(w, "could not unmarshal json traces", http.StatusInternalServerError)
	}

	if len(traces) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	select {
	case c <- traces:
	case <-r.Context().Done():
	}
}

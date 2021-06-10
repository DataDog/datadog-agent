package v0_1_0

import (
	"encoding/json"
	"net/http"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
	"github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1_0/types"
)

func NewServeMux(c agenttypes.RawJSONEventsChan) *http.ServeMux {
	mux := http.NewServeMux()

	s := server{c}
	mux.HandleFunc("/events", s.HandleEvents)
	return mux
}

type server struct {
	c agenttypes.RawJSONEventsChan
}

func (s *server) HandleEvents(w http.ResponseWriter, r *http.Request) {
	switch ct := r.Header.Get("Content-Type"); ct {
	case "application/json":
		handleJSONEvents(w, r, s.c)
	default:
		http.Error(w, "unexpected Content-Type value", http.StatusBadRequest)
	}
}

func handleJSONEvents(w http.ResponseWriter, r *http.Request, c agenttypes.RawJSONEventsChan) {
	var events types.RawJSONEventSlice
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		http.Error(w, "could not unmarshal json traces", http.StatusInternalServerError)
	}

	if len(events) == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	select {
	case c <- events:
	case <-r.Context().Done():
	}
}

// +build linux_bpf

package http

import (
	"C"
	"strings"
	"sync"
)

type httpStatKeeper struct {
	mux   sync.Mutex
	stats map[Key]map[string]RequestStats
}

func newHTTPStatkeeper() *httpStatKeeper {
	return &httpStatKeeper{
		stats: make(map[Key]map[string]RequestStats),
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	h.mux.Lock()
	defer h.mux.Unlock()

	for _, tx := range transactions {
		key := Key{
			SourceIP:   tx.SourceIP(),
			DestIP:     tx.DestIP(),
			SourcePort: tx.SourcePort(),
			DestPort:   tx.DestPort(),
		}
		path := cleanPath(tx.Path())
		statusClass := tx.StatusClass()
		latency := tx.RequestLatency()

		if _, ok := h.stats[key]; !ok {
			h.stats[key] = make(map[string]RequestStats)
		}
		stats := h.stats[key][path]
		stats.AddRequest(statusClass, latency)
		h.stats[key][path] = stats
	}
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]map[string]RequestStats {
	h.mux.Lock()
	defer h.mux.Unlock()

	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]map[string]RequestStats)
	return ret
}

func (h *httpStatKeeper) Close() {}

func cleanPath(path string) string {
	// TODO: remove query variables / redact sensitive information from the request path

	// For now, we'll simply remove queries from the path by stripping anything after a ?
	i := strings.Index(path, "?")
	if i > -1 {
		path = path[:i]
	}

	return path
}

// for testing (needs to be here because cgo cannot be imported into test files)
func makeRequestFragment(path string) [25]C.char {
	fragment := "GET " + path + " HTTP/1.1\nHost: example.com\nUser-Agent: example-browser/1.0"

	var ret [25]C.char
	for i := 0; i < 25; i++ {
		ret[i] = C.char(fragment[i])
	}
	return ret
}

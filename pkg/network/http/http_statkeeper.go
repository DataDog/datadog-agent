// +build linux_bpf

package http

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type Key struct {
	SourceIP util.Address
	DestIP   util.Address
	DestPort uint16
}

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
			SourceIP: tx.SourceIP(),
			DestIP:   tx.DestIP(),
			DestPort: tx.DestPort(),
		}
		path := cleanPath(tx.Path())
		statusClass := tx.StatusClass()
		latency := tx.RequestLatency()

		if _, ok := h.stats[key]; !ok {
			h.stats[key] = make(map[string]RequestStats)
		}
		stats := h.stats[key][path]
		stats.addRequest(statusClass, latency)
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
	// add a flag for aggregation
	return path
}

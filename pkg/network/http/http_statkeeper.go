// +build linux_bpf

package http

import (
	"C"
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
		path := tx.Path()
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

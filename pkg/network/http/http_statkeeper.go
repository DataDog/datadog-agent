// +build linux_bpf

package http

import (
	"C"
)

type httpStatKeeper struct {
	stats  map[Key]map[string]RequestStats
	buffer []byte
}

func newHTTPStatkeeper() *httpStatKeeper {
	return &httpStatKeeper{
		stats:  make(map[Key]map[string]RequestStats),
		buffer: make([]byte, HTTPBufferSize),
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	for _, tx := range transactions {
		key := tx.ToKey()
		path := tx.Path(h.buffer)
		statusClass := tx.StatusClass()
		latency := tx.RequestLatency()

		if _, ok := h.stats[key]; !ok {
			h.stats[key] = make(map[string]RequestStats)
		}
		stats := h.stats[key][string(path)]
		stats.AddRequest(statusClass, latency)
		h.stats[key][string(path)] = stats
	}
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]map[string]RequestStats {
	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]map[string]RequestStats)
	return ret
}

func (h *httpStatKeeper) Close() {}

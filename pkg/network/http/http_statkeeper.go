// +build linux_bpf

package http

import (
	"C"
)

type httpStatKeeper struct {
	stats  map[Key]map[string]RequestStats
	buffer []byte

	// map containing interned path strings
	// this is rotated together with the stats map
	interned map[string]string
}

func newHTTPStatkeeper() *httpStatKeeper {
	return &httpStatKeeper{
		stats:    make(map[Key]map[string]RequestStats),
		buffer:   make([]byte, HTTPBufferSize),
		interned: make(map[string]string),
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	for _, tx := range transactions {
		key := tx.ToKey()
		if _, ok := h.stats[key]; !ok {
			h.stats[key] = make(map[string]RequestStats)
		}

		path := tx.Path(h.buffer)
		statusClass := tx.StatusClass()
		latency := tx.RequestLatency()
		pathString := h.intern(path)
		stats := h.stats[key][pathString]
		stats.AddRequest(statusClass, latency)
		h.stats[key][pathString] = stats
	}
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]map[string]RequestStats {
	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]map[string]RequestStats)
	h.interned = make(map[string]string)
	return ret
}

func (h *httpStatKeeper) intern(b []byte) string {
	v, ok := h.interned[string(b)]
	if !ok {
		v = string(b)
		h.interned[v] = v
	}
	return v
}

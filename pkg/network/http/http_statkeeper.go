// +build linux_bpf

package http

import (
	"C"
)

type httpStatKeeper struct {
	stats map[Key]RequestStats

	// http path buffer
	buffer []byte

	// map containing interned path strings
	// this is rotated  with the stats map
	interned map[string]string
}

func newHTTPStatkeeper() *httpStatKeeper {
	return &httpStatKeeper{
		stats:    make(map[Key]RequestStats),
		buffer:   make([]byte, HTTPBufferSize),
		interned: make(map[string]string),
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	for _, tx := range transactions {
		key := h.newKey(tx)
		stats := h.stats[key]
		stats.AddRequest(tx.StatusClass(), tx.RequestLatency())
		h.stats[key] = stats
	}
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]RequestStats {
	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]RequestStats)
	h.interned = make(map[string]string)
	return ret
}

func (h *httpStatKeeper) newKey(tx httpTX) Key {
	path := tx.Path(h.buffer)
	pathString := h.intern(path)

	return Key{
		SrcIPHigh: uint64(tx.tup.saddr_h),
		SrcIPLow:  uint64(tx.tup.saddr_l),
		SrcPort:   uint16(tx.tup.sport),
		DstIPHigh: uint64(tx.tup.daddr_h),
		DstIPLow:  uint64(tx.tup.daddr_l),
		DstPort:   uint16(tx.tup.dport),
		Path:      pathString,
	}
}

func (h *httpStatKeeper) intern(b []byte) string {
	v, ok := h.interned[string(b)]
	if !ok {
		v = string(b)
		h.interned[v] = v
	}
	return v
}

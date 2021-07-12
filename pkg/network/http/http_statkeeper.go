// +build linux_bpf

package http

import (
	"C"
)
import "sync/atomic"

type httpStatKeeper struct {
	stats      map[Key]RequestStats
	incomplete map[Key]httpTX
	maxEntries int
	telemetry  *telemetry

	// http path buffer
	buffer []byte

	// map containing interned path strings
	// this is rotated  with the stats map
	interned map[string]string
}

func newHTTPStatkeeper(maxEntries int, telemetry *telemetry) *httpStatKeeper {
	return &httpStatKeeper{
		stats:      make(map[Key]RequestStats),
		incomplete: make(map[Key]httpTX),
		maxEntries: maxEntries,
		buffer:     make([]byte, HTTPBufferSize),
		interned:   make(map[string]string),
		telemetry:  telemetry,
	}
}

func (h *httpStatKeeper) Process(transactions []httpTX) {
	for _, tx := range transactions {
		if tx.Incomplete() {
			h.handleIncomplete(tx)
			continue
		}

		h.add(tx)
	}

	atomic.StoreInt64(&h.telemetry.aggregations, int64(len(h.stats)))
}

func (h *httpStatKeeper) GetAndResetAllStats() map[Key]RequestStats {
	ret := h.stats // No deep copy needed since `h.stats` gets reset
	h.stats = make(map[Key]RequestStats)
	h.incomplete = make(map[Key]httpTX)
	h.interned = make(map[string]string)
	return ret
}

func (h *httpStatKeeper) add(tx httpTX) {
	key := h.newKey(tx)
	stats, ok := h.stats[key]
	if !ok && len(h.stats) >= h.maxEntries {
		atomic.AddInt64(&h.telemetry.dropped, 1)
		return
	}

	stats.AddRequest(tx.StatusClass(), tx.RequestLatency())
	h.stats[key] = stats
}

// handleIncomplete is responsible for handling incomplete transactions
// (eg. httpTX objects that have either only the request or response information)
// this happens only in the context of localhost traffic with NAT and these disjoint
// parts of the transactions are joined here by src port
func (h *httpStatKeeper) handleIncomplete(tx httpTX) {
	key := Key{
		SrcIPHigh: uint64(tx.tup.saddr_h),
		SrcIPLow:  uint64(tx.tup.saddr_l),
		SrcPort:   uint16(tx.tup.sport),
	}

	otherHalf, ok := h.incomplete[key]
	if !ok {
		if len(h.incomplete) >= h.maxEntries {
			atomic.AddInt64(&h.telemetry.dropped, 1)
		} else {
			h.incomplete[key] = tx
		}

		return
	}

	request, response := tx, otherHalf
	if response.StatusClass() == 0 {
		request, response = response, request
	}

	// Merge response into request
	request.response_status_code = response.response_status_code
	request.response_last_seen = response.response_last_seen
	h.add(request)
	delete(h.incomplete, key)
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
		Method:    Method(tx.request_method),
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

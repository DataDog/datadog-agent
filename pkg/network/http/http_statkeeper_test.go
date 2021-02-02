// +build linux_bpf

package http

import (
	"encoding/binary"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

func TestProcessHTTPTransactions(t *testing.T) {
	sk := newHTTPStatkeeper()
	txs := make([]httpTX, 100)

	sourceIP := util.AddressFromString("1.1.1.1")
	sourcePort := 1234
	destIP := util.AddressFromString("2.2.2.2")
	destPort := 8080

	for i := 0; i < 10; i++ {
		path := "/testpath" + strconv.Itoa(i)

		for j := 0; j < 10; j++ {
			statusCode := (j%5 + 1) * 100
			latency := float64(j % 5)
			txs[i*10+j] = generateIPv4HTTPTransaction(sourceIP, destIP, sourcePort, destPort, path, statusCode, latency)
		}
	}

	sk.Process(txs)

	stats := sk.GetAndResetAllStats()
	assert.Equal(t, len(sk.stats), 0)

	assert.Equal(t, len(stats), 1)
	for key, statsMap := range stats {
		assert.Equal(t, key, Key{
			SourceIP:   sourceIP,
			SourcePort: uint16(sourcePort),
			DestIP:     destIP,
			DestPort:   uint16(destPort),
		})

		assert.Equal(t, len(statsMap), 10)
		for path, stats := range statsMap {
			assert.Equal(t, "/testpath", path[:9])

			for i := 0; i < 5; i++ {
				assert.Equal(t, 2, stats[i].count)
				assert.Equal(t, 2.0, stats[i].latencies.GetCount())

				p50, err := stats[i].latencies.GetValueAtQuantile(0.5)
				assert.Nil(t, err)

				expectedLatency := float64(i)
				acceptableError := expectedLatency * RelativeAccuracy
				assert.True(t, p50 >= expectedLatency-acceptableError)
				assert.True(t, p50 <= expectedLatency+acceptableError)
			}
		}
	}
}

func TestCleanPath(t *testing.T) {
	path := "/some/path?key1=val1&key2=val2"
	expected := "/some/path"
	assert.Equal(t, expected, cleanPath(path))

	path = "/some/path/with/no/query"
	expected = "/some/path/with/no/query"
	assert.Equal(t, expected, cleanPath(path))
}

func generateIPv4HTTPTransaction(source util.Address, dest util.Address, sourcePort int, destPort int, path string, code int, latency float64) httpTX {
	var tx httpTX

	tx.request_started = 0
	tx.response_last_seen = _Ctype_ulonglong(latency * 1000000.0) // ms to ns
	tx.response_status_code = _Ctype_ushort(code)
	tx.request_fragment = makeRequestFragment(path)
	tx.tup.saddr_l = _Ctype_ulonglong(binary.LittleEndian.Uint32(source.Bytes()))
	tx.tup.sport = _Ctype_ushort(sourcePort)
	tx.tup.daddr_l = _Ctype_ulonglong(binary.LittleEndian.Uint32(dest.Bytes()))
	tx.tup.dport = _Ctype_ushort(destPort)
	tx.tup.metadata = 1

	return tx
}

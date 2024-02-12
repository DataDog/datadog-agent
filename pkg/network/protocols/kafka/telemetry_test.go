package kafka

import (
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

func TestTelemetry_Count(t *testing.T) {
	tests := []struct {
		name string
		tx1  *EbpfTx
		tx2  *EbpfTx
	}{
		{
			name: "Sanity",
			tx1: &EbpfTx{
				Request_api_key:     0,
				Request_api_version: 4,
			},
			tx2: &EbpfTx{
				Request_api_key:     1,
				Request_api_version: 7,
			},
		},
		{
			name: "One unsupported version",
			tx1: &EbpfTx{
				Request_api_key:     0,
				Request_api_version: 0,
			},
			tx2: &EbpfTx{
				Request_api_key:     1,
				Request_api_version: 7,
			},
		},
		{
			name: "Two unsupported version",
			tx1: &EbpfTx{
				Request_api_key:     0,
				Request_api_version: 0,
			},
			tx2: &EbpfTx{
				Request_api_key:     1,
				Request_api_version: 0,
			},
		},
		{
			name: "Unsupported api key",
			tx1: &EbpfTx{
				Request_api_key:     3,
				Request_api_version: 5,
			},
			tx2: &EbpfTx{
				Request_api_key:     1,
				Request_api_version: 8,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetry.Clear()
			tel := NewTelemetry()
			tel.Count(tt.tx1)
			tel.Count(tt.tx2)
			verifyHitsCount(t, tel, tt.tx1)
			verifyHitsCount(t, tel, tt.tx2)
		})
	}
}

func verifyHitsCount(t *testing.T, telemetry *Telemetry, tx *EbpfTx) {
	if tx.Request_api_key == 0 {
		if tx.Request_api_version < minSupportedApiVersion || tx.Request_api_version > maxSupportedApiVersion {
			assert.Equal(t, telemetry.produceHits.hitsUnsupportedVersion.Get(), int64(1), "hitsUnsupportedVersion count is incorrect")
			return
		}
		assert.Equal(t, telemetry.produceHits.hitsVersions[tx.Request_api_version-1].Get(), int64(1), "produceHits count is incorrect")
	} else if tx.Request_api_key == 1 {
		if tx.Request_api_version < minSupportedApiVersion || tx.Request_api_version > maxSupportedApiVersion {
			assert.Equal(t, telemetry.fetchHits.hitsUnsupportedVersion.Get(), int64(1), "hitsUnsupportedVersion count is incorrect")
			return
		}
		assert.Equal(t, telemetry.fetchHits.hitsVersions[tx.Request_api_version-1].Get(), int64(1), "fetchHits count is incorrect")
	}
}

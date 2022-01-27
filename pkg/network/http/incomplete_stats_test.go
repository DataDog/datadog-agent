//go:build linux_bpf
// +build linux_bpf

package http

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrphanEntries(t *testing.T) {
	t.Run("orphan entries can be joined even after flushing", func(t *testing.T) {
		now := time.Now()
		buffer := newIncompleteBuffer(config.New(), newTelemetry())
		request := httpTX{
			request_fragment: requestFragment([]byte("GET /foo/bar")),
			request_started:  _Ctype_ulonglong(now.UnixNano()),
		}
		request.tup.sport = 60000

		buffer.Add(request)
		now = now.Add(5 * time.Second)
		complete := buffer.Flush(now)
		assert.Len(t, complete, 0)

		response := httpTX{
			response_status_code: 200,
			response_last_seen:   _Ctype_ulonglong(now.UnixNano()),
		}
		response.tup.sport = 60000
		buffer.Add(response)
		complete = buffer.Flush(now)
		require.Len(t, complete, 1)

		completeTX := complete[0]
		assert.Equal(t, "/foo/bar", string(completeTX.Path(make([]byte, 256))))
		assert.Equal(t, 200, completeTX.StatusClass())
	})

	t.Run("orphan entries are not kept indefinitely", func(t *testing.T) {
		buffer := newIncompleteBuffer(config.New(), newTelemetry())
		now := time.Now()
		buffer.minAgeNano = (30 * time.Second).Nanoseconds()
		request := httpTX{
			request_fragment: requestFragment([]byte("GET /foo/bar")),
			request_started:  _Ctype_ulonglong(now.UnixNano()),
		}
		buffer.Add(request)
		_ = buffer.Flush(now)

		assert.True(t, len(buffer.data) > 0)
		now = now.Add(35 * time.Second)
		_ = buffer.Flush(now)
		assert.True(t, len(buffer.data) == 0)
	})
}

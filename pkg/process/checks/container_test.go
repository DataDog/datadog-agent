// +build linux

package checks

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
)

func TestContainerChunking(t *testing.T) {
	ctrs := []*containers.Container{
		makeContainer("foo"),
		makeContainer("bar"),
		makeContainer("bim"),
	}
	lastRun := time.Now().Add(-5 * time.Second)

	for i, tc := range []struct {
		cur      []*containers.Container
		last     map[string]util.ContainerRateMetrics
		chunks   int
		expected int
		maxSize  int
	}{
		{
			cur:      []*containers.Container{ctrs[0], ctrs[1], ctrs[2]},
			last:     util.ExtractContainerRateMetric([]*containers.Container{ctrs[0], ctrs[1], ctrs[2]}),
			chunks:   2,
			expected: 3,
			maxSize:  2,
		},
		{
			cur:      []*containers.Container{ctrs[0], ctrs[1], ctrs[2]},
			last:     util.ExtractContainerRateMetric([]*containers.Container{ctrs[0], ctrs[2]}),
			chunks:   2,
			expected: 3,
			maxSize:  2,
		},
		{
			cur:      []*containers.Container{ctrs[0], ctrs[2]},
			last:     util.ExtractContainerRateMetric([]*containers.Container{ctrs[0], ctrs[1], ctrs[2]}),
			chunks:   2,
			expected: 2,
			maxSize:  1,
		},
	} {
		chunked := chunkContainers(tc.cur, tc.last, lastRun, tc.chunks, tc.maxSize)
		assert.Len(t, chunked, tc.chunks, "len test %d", i)
		total := 0
		for _, c := range chunked {
			total += len(c)
		}
		assert.Equal(t, tc.expected, total, "total test %d", i)

		chunkedStat := fmtContainerStats(tc.cur, tc.last, lastRun, tc.chunks)
		assert.Len(t, chunkedStat, tc.chunks, "len stat test %d", i)
		total = 0
		for _, c := range chunked {
			total += len(c)
		}
		assert.Equal(t, tc.expected, total, "total test %d", i)

	}
}

func TestContainerAddresses(t *testing.T) {
	ctr := makeContainer("haha")
	ctr.AddressList = []containers.NetworkAddress{{IP: net.ParseIP("192.168.128.141"), Port: 443, Protocol: "TCP"}}
	results := fmtContainers([]*containers.Container{ctr}, map[string]util.ContainerRateMetrics{}, time.Now())
	assert.Equal(t, 1, len(results))
	addrs := []*model.ContainerAddr{
		{Ip: "192.168.128.141", Port: int32(443), Protocol: model.ConnectionType_tcp},
	}
	assert.Equal(t, results[0].Addresses, addrs)
}

func TestContainerNils(t *testing.T) {
	// Make sure formatting doesn't crash with nils
	cur := []*containers.Container{{}}
	last := map[string]util.ContainerRateMetrics{}
	chunkContainers(cur, last, time.Now(), 10, 10)
	fmtContainerStats(cur, last, time.Now(), 10)
	// Make sure we get values when we have nils in last.
	cur = []*containers.Container{
		{
			ID: "1",
			ContainerMetrics: metrics.ContainerMetrics{
				CPU: &metrics.ContainerCPUStats{},
			},
		},
	}
	last = map[string]util.ContainerRateMetrics{
		"1": {
			CPU: &metrics.ContainerCPUStats{},
		},
	}
	chunkContainers(cur, last, time.Now(), 10, 10)
	fmtContainerStats(cur, last, time.Now(), 10)
}

func TestCalculateCtrPct(t *testing.T) {
	epsilon := 0.0000001 // Difference less than some epsilon

	before := time.Now().Add(-1 * time.Second)

	var emptyTime time.Time

	// Underflow on cur-prev
	assert.Equal(t, float32(0), calculateCtrPct(0, 1, 0, 0, 1, before))

	// Underflow on sys2-sys1
	assert.Equal(t, float32(0), calculateCtrPct(3, 1, 4, 5, 1, before))

	// Time is empty
	assert.Equal(t, float32(0), calculateCtrPct(3, 1, 0, 0, 1, emptyTime))

	// Elapsed time is less than 1s
	assert.Equal(t, float32(0), calculateCtrPct(3, 1, 0, 0, 1, time.Now()))

	// Div by zero on sys2/sys1, fallback to normal cpu calculation
	assert.InEpsilon(t, 2, calculateCtrPct(3, 1, 1, 1, 1, before), epsilon)

	// Calculate based off cur & prev
	assert.InEpsilon(t, 2, calculateCtrPct(3, 1, 0, 0, 1, before), epsilon)

	// Calculate based off all values
	assert.InEpsilon(t, 66.66667, calculateCtrPct(3, 1, 4, 1, 1, before), epsilon)
}

package host

import (
	"testing"

	"github.com/shirou/gopsutil/host"
	"github.com/stretchr/testify/assert"
)

func TestFillOsVersion(t *testing.T) {
	stats := &systemStats{}
	info, _ := host.Info()
	fillOsVersion(stats, info)
	assert.Len(t, stats.Nixver, 3)
	assert.NotEmpty(t, stats.Nixver[0])
	assert.NotEmpty(t, stats.Nixver[1])
	assert.Empty(t, stats.Nixver[2])
}

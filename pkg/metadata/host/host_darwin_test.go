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
	assert.NotEmpty(t, stats.Macver.Release)
}

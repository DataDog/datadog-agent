package host

import (
	"testing"

	"github.com/shirou/gopsutil/host"
)

func TestFillOsVersion(t *testing.T) {
	stats := &systemStats{}
	info, _ := host.Info()
	fillOsVersion(stats, info)
}

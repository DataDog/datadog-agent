package metadata

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestProcessMetadataProvider_ParseMetadata(t *testing.T) {
	pmp := NewProcessMetadataProvider()

	pmp.Extract(processData())
}

func processData() map[int32]*procutil.Process {
	return map[int32]*procutil.Process{
		1: {
			Pid: 1,
			Cmdline: []string{
				"/sbin/init",
			},
		},
		23211: {
			Pid: 23211,
			Cmdline: []string{
				"/usr/bin/docker-proxy",
				"-proto",
				"tcp",
				"-host-ip",
				"0.0.0.0",
				"-host-port",
				"32769",
				"-container-ip",
				"172.17.0.2",
				"-container-port",
				"6379",
			},
		},
	}
}

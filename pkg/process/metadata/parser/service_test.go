package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestExtractServiceMetadata(t *testing.T) {
	tests := []struct {
		name               string
		cmdline            []string
		expectedServiceTag string
	}{
		{
			name:               "empty",
			cmdline:            []string{},
			expectedServiceTag: "",
		},
		{
			name:               "blank",
			cmdline:            []string{""},
			expectedServiceTag: "",
		},
		{
			name: "single arg executable",
			cmdline: []string{
				"./my-server.sh",
			},
			expectedServiceTag: "service:my-server",
		},
		{
			name: "sudo",
			cmdline: []string{
				"sudo", "-E", "-u", "dog", "/usr/local/bin/myApp", "-items=0,1,2,3", "-foo=bar",
			},
			expectedServiceTag: "service:myApp",
		},
		{
			name: "python flask argument",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "flask", "run", "--host=0.0.0.0",
			},
			expectedServiceTag: "service:flask",
		},
		{
			name: "python - flask argument in path",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "/opt/dogweb/bin/flask", "run", "--host=0.0.0.0", "--without-threads",
			},
			expectedServiceTag: "service:flask",
		},
		{
			name: "ruby - td-agent",
			cmdline: []string{
				"ruby", "/usr/sbin/td-agent", "--log", "/var/log/td-agent/td-agent.log", "--daemon", "/var/run/td-agent/td-agent.pid ",
			},
			expectedServiceTag: "service:td-agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := procutil.Process{
				Pid:     1,
				Cmdline: tt.cmdline,
			}
			d := NewServiceExtractor()

			d.Extract(&proc)
			assert.Equal(t, tt.expectedServiceTag, d.GetServiceTag(proc.Pid))
		})
	}
}

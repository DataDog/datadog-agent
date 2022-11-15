package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestExtractServiceMetadata(t *testing.T) {
	tests := []struct {
		name            string
		cmdline         []string
		expectedService string
	}{
		{
			name:            "empty",
			cmdline:         []string{},
			expectedService: "",
		},
		{
			name:            "blank",
			cmdline:         []string{""},
			expectedService: "",
		},
		{
			name: "single arg executable",
			cmdline: []string{
				"./my-server.sh",
			},
			expectedService: "my-server",
		},
		{
			name: "sudo",
			cmdline: []string{
				"sudo", "-E", "-u", "dog", "/usr/local/bin/myApp", "-items=0,1,2,3", "-foo=bar",
			},
			expectedService: "myApp",
		},
		{
			name: "python flask argument",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "flask", "run", "--host=0.0.0.0",
			},
			expectedService: "flask",
		},
		{
			name: "python - flask argument in path",
			cmdline: []string{
				"/opt/python/2.7.11/bin/python2.7", "/opt/dogweb/bin/flask", "run", "--host=0.0.0.0", "--without-threads",
			},
			expectedService: "flask",
		},
		{
			name: "ruby - td-agent",
			cmdline: []string{
				"ruby", "/usr/sbin/td-agent", "--log", "/var/log/td-agent/td-agent.log", "--daemon", "/var/run/td-agent/td-agent.pid ",
			},
			expectedService: "td-agent",
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
			assert.Equal(t, tt.expectedService, d.GetServiceTag(proc.Pid))
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/discovery/envs"
)

func Test_detectErlangAppName(t *testing.T) {
	tests := []struct {
		name     string
		cmdline  []string
		expected string
	}{
		{
			name: "CouchDB with progname",
			cmdline: []string{
				"-root", "/usr/lib/erlang",
				"-progname", "couchdb",
				"-home", "/opt/couchdb",
			},
			expected: "couchdb",
		},
		{
			name: "Riak with progname",
			cmdline: []string{
				"-root", "/usr/lib/erlang",
				"-progname", "riak",
				"-home", "/var/lib/riak",
			},
			expected: "riak",
		},
		{
			name: "RabbitMQ with erl progname, use home",
			cmdline: []string{
				"-root", "/usr/lib/erlang",
				"-progname", "erl",
				"-home", "/var/lib/rabbitmq",
			},
			expected: "rabbitmq",
		},
		{
			name: "Ejabberd with erl progname, use home",
			cmdline: []string{
				"-root", "/usr/lib/erlang",
				"-progname", "erl",
				"-home", "/var/lib/ejabberd",
			},
			expected: "ejabberd",
		},
		{
			name: "Generic Erlang process with erl progname, use home",
			cmdline: []string{
				"-progname", "erl",
				"-home", "/usr/local/myapp",
			},
			expected: "myapp",
		},
		{
			name: "No progname or home returns empty string",
			cmdline: []string{
				"-smp", "auto",
				"-noinput",
			},
			expected: "",
		},
		{
			name: "Progname without home",
			cmdline: []string{
				"-progname", "myerlangapp",
				"-smp", "auto",
			},
			expected: "myerlangapp",
		},
		{
			name:     "Empty command line",
			cmdline:  []string{},
			expected: "",
		},
		{
			name: "Only home, no progname",
			cmdline: []string{
				"-home", "/opt/customapp",
				"-noshell",
			},
			expected: "",
		},
		{
			name: "Home with trailing slash",
			cmdline: []string{
				"-progname", "erl",
				"-home", "/var/lib/myapp/",
			},
			expected: "myapp",
		},
		{
			name: "Complex real-world RabbitMQ command line",
			cmdline: []string{
				"-W", "w",
				"-MBas", "ageffcbf",
				"-MHas", "ageffcbf",
				"-MBlmbcs", "512",
				"-MHlmbcs", "512",
				"-MMmcs", "30",
				"-P", "1048576",
				"-t", "5000000",
				"-stbt", "db",
				"-zdbbl", "128000",
				"-sbwt", "none",
				"-sbwtdcpu", "none",
				"-sbwtdio", "none",
				"-K", "true",
				"-A", "192",
				"-sdio", "192",
				"-kernel", "inet_dist_listen_min", "25672",
				"-kernel", "inet_dist_listen_max", "25672",
				"-kernel", "shell_history", "enabled",
				"-boot", "/usr/lib/rabbitmq/bin/../releases/3.11.5/start_clean",
				"-lager", "crash_log", "false",
				"-lager", "handlers", "[]",
				"-rabbit", "product_name", "\"RabbitMQ\"",
				"-rabbit", "product_version", "\"3.11.5\"",
				"-progname", "erl",
				"-home", "/var/lib/rabbitmq",
			},
			expected: "rabbitmq",
		},
		{
			name: "CouchDB real-world command line",
			cmdline: []string{
				"-noshell",
				"-noinput",
				"+Bd",
				"-B",
				"-K", "true",
				"-A", "16",
				"-n",
				"+A", "4",
				"+sbtu",
				"+sbwt", "none",
				"+sbwtdcpu", "none",
				"+sbwtdio", "none",
				"-config", "/opt/couchdb/releases/3.2.2/sys.config",
				"-sasl", "errlog_type", "error",
				"-couch_ini", "/opt/couchdb/etc/default.ini", "/opt/couchdb/etc/local.ini",
				"-boot", "/opt/couchdb/releases/3.2.2/couchdb",
				"-args_file", "/opt/couchdb/etc/vm.args",
				"-progname", "couchdb",
			},
			expected: "couchdb",
		},
		{
			name: "Progname with spaces in value",
			cmdline: []string{
				"-progname", "  couchdb  ",
			},
			expected: "couchdb",
		},
		{
			name: "Home with spaces in value",
			cmdline: []string{
				"-progname", "erl",
				"-home", "  /var/lib/rabbitmq  ",
			},
			expected: "rabbitmq",
		},
		{
			name: "Uppercase ERL is not treated as erl",
			cmdline: []string{
				"-progname", "ERL",
				"-home", "/var/lib/rabbitmq",
			},
			expected: "ERL",
		},
		{
			name: "Mixed case Erl is not treated as erl",
			cmdline: []string{
				"-progname", "Erl",
				"-home", "/var/lib/ejabberd",
			},
			expected: "Erl",
		},
		{
			name: "progname as last argument without value",
			cmdline: []string{
				"-root", "/usr/lib/erlang",
				"-home", "/var/lib/myapp",
				"-progname",
			},
			expected: "",
		},
		{
			name: "home as last argument without value",
			cmdline: []string{
				"-root", "/usr/lib/erlang",
				"-progname", "erl",
				"-home",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectErlangAppName(tt.cmdline)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_erlangDetector(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedName    string
		expectedSuccess bool
		expectedSource  ServiceNameSource
	}{
		{
			name: "CouchDB detection",
			args: []string{
				"-progname", "couchdb",
				"-home", "/opt/couchdb",
			},
			expectedName:    "couchdb",
			expectedSuccess: true,
			expectedSource:  CommandLine,
		},
		{
			name: "RabbitMQ detection",
			args: []string{
				"-progname", "erl",
				"-home", "/var/lib/rabbitmq",
			},
			expectedName:    "rabbitmq",
			expectedSuccess: true,
			expectedSource:  CommandLine,
		},
		{
			name: "No name extracted returns false",
			args: []string{
				"-smp", "auto",
			},
			expectedName:    "",
			expectedSuccess: false,
		},
		{
			name: "Custom Erlang app",
			args: []string{
				"-progname", "myapp",
				"-noshell",
			},
			expectedName:    "myapp",
			expectedSuccess: true,
			expectedSource:  CommandLine,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewDetectionContext(tt.args, envs.NewVariables(nil), nil)
			detector := newErlangDetector(ctx)

			meta, success := detector.detect(tt.args)

			assert.Equal(t, tt.expectedSuccess, success)
			if success {
				assert.Equal(t, tt.expectedName, meta.Name)
				assert.Equal(t, tt.expectedSource, meta.Source)
			}
		})
	}
}

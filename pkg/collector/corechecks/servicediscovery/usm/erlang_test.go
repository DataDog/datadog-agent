// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
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
			name: "Fallback to beam when no progname or home",
			cmdline: []string{
				"-smp", "auto",
				"-noinput",
			},
			expected: "beam",
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
			expected: "beam",
		},
		{
			name: "Only home, no progname",
			cmdline: []string{
				"-home", "/opt/customapp",
				"-noshell",
			},
			expected: "beam",
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
			name: "Progname concatenated without space",
			cmdline: []string{
				"-root", "/usr/lib/erlang",
				"-prognamecouchdb",
				"-home", "/opt/couchdb",
			},
			expected: "couchdb",
		},
		{
			name: "Home concatenated without space",
			cmdline: []string{
				"-progname", "erl",
				"-home/var/lib/rabbitmq",
			},
			expected: "rabbitmq",
		},
		{
			name: "Both flags concatenated",
			cmdline: []string{
				"-prognameerl",
				"-home/var/lib/ejabberd",
			},
			expected: "ejabberd",
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
			name: "Case insensitive erl - uppercase ERL",
			cmdline: []string{
				"-progname", "ERL",
				"-home", "/var/lib/rabbitmq",
			},
			expected: "rabbitmq",
		},
		{
			name: "Case insensitive erl - mixed case Erl",
			cmdline: []string{
				"-progname", "Erl",
				"-home", "/var/lib/ejabberd",
			},
			expected: "ejabberd",
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
			name: "Fallback to beam returns false",
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

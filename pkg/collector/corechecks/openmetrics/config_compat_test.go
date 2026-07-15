// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const compatTestInstance = `
openmetrics_endpoint: http://127.0.0.1:9090/metrics
metrics: []
`

func TestParseConfigWithInitInheritance(t *testing.T) {
	tests := []struct {
		name       string
		init       string
		instance   string
		wantError  bool
		assertions func(*testing.T, *scraperConfig)
	}{
		{
			name: "timeout inherited when absent",
			init: "timeout: 23\n",
			assertions: func(t *testing.T, cfg *scraperConfig) {
				require.Equal(t, 23*time.Second, cfg.timeout)
			},
		},
		{
			name:     "instance timeout wins",
			init:     "timeout: 23\n",
			instance: "timeout: 7\n",
			assertions: func(t *testing.T, cfg *scraperConfig) {
				require.Equal(t, 7*time.Second, cfg.timeout)
			},
		},
		{
			name: "skip proxy inherited when absent",
			init: "skip_proxy: true\n",
			assertions: func(t *testing.T, cfg *scraperConfig) {
				require.True(t, cfg.skipProxy)
			},
		},
		{
			name:     "instance skip proxy wins",
			init:     "skip_proxy: true\n",
			instance: "skip_proxy: false\n",
			assertions: func(t *testing.T, cfg *scraperConfig) {
				require.False(t, cfg.skipProxy)
			},
		},
		{
			name:      "log requests inherited when absent",
			init:      "log_requests: true\n",
			wantError: true,
		},
		{
			name:     "instance log requests wins",
			init:     "log_requests: true\n",
			instance: "log_requests: false\n",
		},
		{
			name:      "TLS warning setting inherited when absent",
			init:      "tls_ignore_warning: true\n",
			wantError: true,
		},
		{
			name:     "instance TLS warning setting wins",
			init:     "tls_ignore_warning: true\n",
			instance: "tls_ignore_warning: false\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := parseConfigWithInit([]byte(compatTestInstance+test.instance), []byte(test.init))
			if test.wantError {
				require.ErrorIs(t, err, errUnsupportedCoreConfig)
				return
			}

			require.NoError(t, err)
			if test.assertions != nil {
				test.assertions(t, cfg)
			}
		})
	}
}

func TestParseConfigWithInitProxyPrecedence(t *testing.T) {
	const initConfig = `
proxy:
  http: http://init-http:8080
  https: http://init-https:8443
  no_proxy:
    - init.example.com
`
	tests := []struct {
		name        string
		instance    string
		wantProxy   map[string]string
		wantNoProxy []string
	}{
		{
			name: "init proxy used when instance proxy absent",
			wantProxy: map[string]string{
				"http":  "http://init-http:8080",
				"https": "http://init-https:8443",
			},
			wantNoProxy: []string{"init.example.com"},
		},
		{
			name: "instance proxy wholly wins",
			instance: `
proxy:
  http: http://instance:3128
`,
			wantProxy: map[string]string{"http": "http://instance:3128"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := parseConfigWithInit([]byte(compatTestInstance+test.instance), []byte(initConfig))
			require.NoError(t, err)
			require.Equal(t, test.wantProxy, cfg.proxy)
			require.Equal(t, test.wantNoProxy, cfg.noProxy)
		})
	}
}

func TestParseConfigWithInitDoesNotInheritInstanceOnlyHTTPSettings(t *testing.T) {
	initConfig := []byte(`
headers:
  X-Init: init
extra_headers:
  X-Extra-Init: init
username: init-user
password: init-password
auth_type: digest
bearer_token_auth: true
bearer_token_path: /init/token
auth_token:
  reader:
    type: file
    path: /init/auth-token
tls_verify: false
tls_cert: /init/cert
tls_private_key: /init/key
tls_ca_cert: /init/ca
tls_use_host_header: true
tls_protocols_allowed:
  - TLSv1.2
tls_ciphers: PSK-CAMELLIA128-SHA256
allow_redirects: false
persist_connections: true
use_legacy_auth_encoding: false
`)

	cfg, err := parseConfigWithInit([]byte(compatTestInstance), initConfig)
	require.NoError(t, err)
	require.Empty(t, cfg.headers)
	require.Empty(t, cfg.username)
	require.Empty(t, cfg.password)
	require.False(t, cfg.bearerTokenAuth)
	require.Empty(t, cfg.bearerTokenPath)
	require.Nil(t, cfg.authToken)
	require.True(t, cfg.tlsVerify)
	require.Empty(t, cfg.tlsCert)
	require.Empty(t, cfg.tlsPrivateKey)
	require.Empty(t, cfg.tlsCACert)
	require.False(t, cfg.tlsUseHostHeader)
	require.Nil(t, cfg.tlsProtocolsAllowed)
	require.Nil(t, cfg.tlsCiphers)
	require.True(t, cfg.allowRedirect)
	require.False(t, cfg.persistConnections)
	require.True(t, cfg.legacyAuthEncoding)
}

func TestParseConfigCompatibilityDefaults(t *testing.T) {
	tests := []struct {
		name                   string
		instance               string
		wantAllowRedirect      bool
		wantPersistConnections bool
	}{
		{
			name:              "defaults",
			wantAllowRedirect: true,
		},
		{
			name:              "redirects explicitly disabled",
			instance:          "allow_redirects: false\n",
			wantAllowRedirect: false,
		},
		{
			name:                   "persistence explicitly enabled",
			instance:               "persist_connections: true\n",
			wantAllowRedirect:      true,
			wantPersistConnections: true,
		},
		{
			name:                   "TLS host header forces persistence",
			instance:               "tls_use_host_header: true\npersist_connections: false\n",
			wantAllowRedirect:      true,
			wantPersistConnections: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := parseConfig([]byte(compatTestInstance + test.instance))
			require.NoError(t, err)
			require.Equal(t, test.wantAllowRedirect, cfg.allowRedirect)
			require.Equal(t, test.wantPersistConnections, cfg.persistConnections)
		})
	}
}

func TestParseConfigUnsupportedCompatibilitySettings(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "metric patterns", config: "metric_patterns: {}\n"},
		{name: "connect timeout", config: "connect_timeout: 1\n"},
		{name: "read timeout", config: "read_timeout: 1\n"},
		{name: "request size", config: "request_size: 16\n"},
		{name: "TLS protocols", config: "tls_protocols_allowed: [TLSv1.2]\n"},
		{name: "log requests", config: "log_requests: true\n"},
		{name: "ignore TLS warning", config: "tls_ignore_warning: true\n"},
		{name: "advanced auth type", config: "auth_type: ntlm\n"},
		{name: "NTLM domain", config: "ntlm_domain: example.com\n"},
		{name: "AWS host", config: "aws_host: example.com\n"},
		{name: "AWS region", config: "aws_region: us-east-1\n"},
		{name: "AWS service", config: "aws_service: execute-api\n"},
		{name: "Kerberos auth", config: "kerberos_auth: required\n"},
		{name: "Kerberos cache", config: "kerberos_cache: /tmp/krb5cc\n"},
		{name: "Kerberos delegation", config: "kerberos_delegate: true\n"},
		{name: "Kerberos force initiate", config: "kerberos_force_initiate: true\n"},
		{name: "Kerberos hostname", config: "kerberos_hostname: host.example.com\n"},
		{name: "Kerberos keytab", config: "kerberos_keytab: /tmp/service.keytab\n"},
		{name: "Kerberos principal", config: "kerberos_principal: service@example.com\n"},
		{name: "generic tags disabled", config: "disable_generic_tags: true\n"},
		{name: "legacy tag normalization disabled", config: "enable_legacy_tags_normalization: false\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseConfig([]byte(compatTestInstance + test.config))
			require.ErrorIs(t, err, errUnsupportedCoreConfig)
		})
	}
}

func TestParseConfigFractionalTimeout(t *testing.T) {
	cfg, err := parseConfig([]byte(compatTestInstance + "timeout: 1.25\n"))
	require.NoError(t, err)
	require.Equal(t, 1250*time.Millisecond, cfg.timeout)
}

func TestParseConfigUnsupportedTLSCipher(t *testing.T) {
	_, err := parseConfig([]byte(compatTestInstance + "tls_ciphers: PSK-CAMELLIA128-SHA256\n"))
	require.ErrorIs(t, err, errUnsupportedCoreConfig)
}

func TestParseConfigLegacyAuthEncoding(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		want     bool
	}{
		{name: "defaults true", want: true},
		{name: "explicit false", instance: "use_legacy_auth_encoding: false\n", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, err := parseConfig([]byte(compatTestInstance + test.instance))
			require.NoError(t, err)
			require.Equal(t, test.want, cfg.legacyAuthEncoding)
		})
	}
}

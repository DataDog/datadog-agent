// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// testSMBPassword looks like an Azure storage account key: base64 with '+', '/' and '='.
const testSMBPassword = "s3cr3t+Key/AbC=="

func validSMBLogsConfig() *LogsConfig {
	return &LogsConfig{
		Type:    SMBType,
		Path:    "app/*.log",
		Service: "demo-app",
		Source:  "demo",
		SMB: &SMBConfig{
			Host:     "myacct.file.core.windows.net",
			Share:    "logs",
			Username: "myacct",
			Password: testSMBPassword,
		},
	}
}

func TestValidateSMB(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *LogsConfig)
		wantErr string // empty means valid
	}{
		{name: "minimal", mutate: func(*LogsConfig) {}},
		{name: "all optional fields", mutate: func(c *LogsConfig) {
			c.SMB.Domain = "CORP"
			c.SMB.Port = 1445
			c.SMB.PollInterval = 0.5
			c.TailingMode = "beginning"
		}},
		{name: "start_position end", mutate: func(c *LogsConfig) { c.TailingMode = "end" }},
		{name: "empty password", mutate: func(c *LogsConfig) { c.SMB.Password = "" }},
		{name: "ipv6 host", mutate: func(c *LogsConfig) { c.SMB.Host = "fd00::10" }},
		{name: "port 0 means default", mutate: func(c *LogsConfig) { c.SMB.Port = 0 }},
		{name: "port max", mutate: func(c *LogsConfig) { c.SMB.Port = 65535 }},
		{name: "file at share root", mutate: func(c *LogsConfig) { c.Path = "app.log" }},
		{name: "nested glob", mutate: func(c *LogsConfig) { c.Path = "app/*/[a-z]?.log" }},
		{name: "dots in names", mutate: func(c *LogsConfig) { c.Path = "app/..hidden/a..b.log" }},
		{name: "valid excludes", mutate: func(c *LogsConfig) { c.ExcludePaths = []string{"app/old-*.log", "app/archive/*"} }},
		{name: "poll_interval minimum", mutate: func(c *LogsConfig) { c.SMB.PollInterval = 0.1 }},
		{name: "poll_interval maximum", mutate: func(c *LogsConfig) { c.SMB.PollInterval = 3600 }},
		{name: "allow_smb2", mutate: func(c *LogsConfig) { c.SMB.AllowSMB2 = true }},
		{name: "require_encryption", mutate: func(c *LogsConfig) { c.SMB.RequireEncryption = true }},
		{name: "allow_smb2 with require_encryption", mutate: func(c *LogsConfig) { c.SMB.AllowSMB2, c.SMB.RequireEncryption = true, true }, wantErr: "require_encryption needs SMB 3 and cannot be combined with allow_smb2"},

		{name: "missing smb block", mutate: func(c *LogsConfig) { c.SMB = nil }, wantErr: "must have an smb block"},
		{name: "smb block on file source", mutate: func(c *LogsConfig) { c.Type = FileType; c.Path = "/var/log/a.log" }, wantErr: "only supported for smb sources, got file"},
		{name: "smb block on docker source", mutate: func(c *LogsConfig) { c.Type = DockerType }, wantErr: "only supported for smb sources, got docker"},
		{name: "missing host", mutate: func(c *LogsConfig) { c.SMB.Host = "" }, wantErr: "must have a host"},
		{name: "host with scheme", mutate: func(c *LogsConfig) { c.SMB.Host = "smb://myacct.file.core.windows.net" }, wantErr: "bare hostname"},
		{name: "host with user info", mutate: func(c *LogsConfig) { c.SMB.Host = "myacct:" + testSMBPassword + "@myacct.file.core.windows.net" }, wantErr: "bare hostname"},
		{name: "host with port", mutate: func(c *LogsConfig) { c.SMB.Host = "myacct.file.core.windows.net:445" }, wantErr: "use the port setting"},
		{name: "host in UNC form", mutate: func(c *LogsConfig) { c.SMB.Host = `\\myacct.file.core.windows.net` }, wantErr: "bare hostname"},
		{name: "missing share", mutate: func(c *LogsConfig) { c.SMB.Share = "" }, wantErr: "must have a share"},
		{name: "share with directory", mutate: func(c *LogsConfig) { c.SMB.Share = "logs/app" }, wantErr: "put directories in path"},
		{name: "missing username", mutate: func(c *LogsConfig) { c.SMB.Username = "" }, wantErr: "must have a username"},
		{name: "unresolved secret handle", mutate: func(c *LogsConfig) { c.SMB.Password = "ENC[smb_account_key]" }, wantErr: "unresolved ENC[] secret handle"},
		{name: "negative port", mutate: func(c *LogsConfig) { c.SMB.Port = -1 }, wantErr: "port must be between 0 and 65535"},
		{name: "port too large", mutate: func(c *LogsConfig) { c.SMB.Port = 65536 }, wantErr: "port must be between 0 and 65535"},
		{name: "negative poll_interval", mutate: func(c *LogsConfig) { c.SMB.PollInterval = -1 }, wantErr: "poll_interval"},
		{name: "NaN poll_interval", mutate: func(c *LogsConfig) { c.SMB.PollInterval = math.NaN() }, wantErr: "poll_interval"},
		{name: "infinite poll_interval", mutate: func(c *LogsConfig) { c.SMB.PollInterval = math.Inf(1) }, wantErr: "poll_interval"},
		// A sub-nanosecond interval truncates to a zero duration and an overflowing one turns
		// negative: either would make the scan ticker panic.
		{name: "sub-nanosecond poll_interval", mutate: func(c *LogsConfig) { c.SMB.PollInterval = 1e-10 }, wantErr: "between 0.1 and 3600 seconds"},
		{name: "poll_interval below the minimum", mutate: func(c *LogsConfig) { c.SMB.PollInterval = 0.05 }, wantErr: "poll_interval"},
		{name: "overflowing poll_interval", mutate: func(c *LogsConfig) { c.SMB.PollInterval = 1e10 }, wantErr: "poll_interval"},
		{name: "poll_interval above the maximum", mutate: func(c *LogsConfig) { c.SMB.PollInterval = 3601 }, wantErr: "poll_interval"},
		{name: "missing path", mutate: func(c *LogsConfig) { c.Path = "" }, wantErr: "must have a path"},
		{name: "absolute path", mutate: func(c *LogsConfig) { c.Path = "/app/*.log" }, wantErr: "relative to the share root"},
		{name: "backslash path", mutate: func(c *LogsConfig) { c.Path = `app\*.log` }, wantErr: "'/' separators"},
		{name: "malformed glob", mutate: func(c *LogsConfig) { c.Path = "app/[.log" }, wantErr: "not a valid glob pattern"},
		{name: "path leaving its directory", mutate: func(c *LogsConfig) { c.Path = "app/../other/*.log" }, wantErr: "must not contain '..' elements"},
		{name: "path leaving the share", mutate: func(c *LogsConfig) { c.Path = "../logs/*.log" }, wantErr: "must not contain '..' elements"},
		{name: "path with a NUL byte", mutate: func(c *LogsConfig) { c.Path = "app/a\x00.log" }, wantErr: "NUL"},
		{name: "invalid exclude glob", mutate: func(c *LogsConfig) { c.ExcludePaths = []string{"app/[old.log"} }, wantErr: `exclude_paths entry "app/[old.log" is not a valid glob pattern`},
		{name: "absolute exclude", mutate: func(c *LogsConfig) { c.ExcludePaths = []string{"/app/old.log"} }, wantErr: "exclude_paths entry \"/app/old.log\" must be relative to the share root"},
		{name: "exclude with ..", mutate: func(c *LogsConfig) { c.ExcludePaths = []string{"app/../x"} }, wantErr: "must not contain '..' elements"},
		{name: "unsupported start_position", mutate: func(c *LogsConfig) { c.TailingMode = "forceBeginning" }, wantErr: "supported: beginning, end"},
		{name: "unknown start_position", mutate: func(c *LogsConfig) { c.TailingMode = "middle" }, wantErr: "invalid start_position"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validSMBLogsConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			// validation errors are shown in agent status and sent as inventory metadata
			assert.NotContains(t, err.Error(), testSMBPassword)
		})
	}
}

func TestValidateSMBStillValidatesCommonSettings(t *testing.T) {
	// An smb source goes through the rest of Validate, so its processing rules get compiled.
	cfg := validSMBLogsConfig()
	cfg.ProcessingRules = []*ProcessingRule{{Name: "drop_debug", Type: ExcludeAtMatch, Pattern: "DEBUG"}}
	require.NoError(t, cfg.Validate())
	assert.NotNil(t, cfg.ProcessingRules[0].Regex)

	cfg.ProcessingRules = []*ProcessingRule{{Name: "bad", Type: ExcludeAtMatch, Pattern: "("}}
	assert.Error(t, cfg.Validate())

	cfg = validSMBLogsConfig()
	cfg.TLS = &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key"}
	assert.ErrorContains(t, cfg.Validate(), "only supported for tcp")
}

func TestParseSMBConfig(t *testing.T) {
	expected := &SMBConfig{
		Host:         "myacct.file.core.windows.net",
		Share:        "logs",
		Username:     "myacct",
		Password:     "ENC[smb_account_key]",
		Domain:       "CORP",
		Port:         1445,
		PollInterval: 0.5,
	}
	expectedStrict := *expected
	expectedStrict.AllowSMB2 = false
	expectedStrict.RequireEncryption = true

	t.Run("yaml", func(t *testing.T) {
		configs, err := ParseYAML([]byte(`
logs:
  - type: smb
    path: "app/*.log"
    service: demo-app
    source: demo
    start_position: beginning
    smb:
      host: myacct.file.core.windows.net
      share: logs
      username: myacct
      password: "ENC[smb_account_key]"
      domain: CORP
      port: 1445
      poll_interval: 0.5
`))
		require.NoError(t, err)
		require.Len(t, configs, 1)
		cfg := configs[0]
		assert.Equal(t, SMBType, cfg.Type)
		assert.Equal(t, "app/*.log", cfg.Path)
		assert.Equal(t, "beginning", cfg.TailingMode)
		// the ENC[] handle is kept verbatim for the secret backend to resolve
		assert.Equal(t, expected, cfg.SMB)
	})

	t.Run("json", func(t *testing.T) {
		configs, err := ParseJSON([]byte(`[{"type":"smb","path":"app/*.log","service":"demo-app","source":"demo","start_position":"beginning",` +
			`"smb":{"host":"myacct.file.core.windows.net","share":"logs","username":"myacct","password":"ENC[smb_account_key]","domain":"CORP","port":1445,"poll_interval":0.5}}]`))
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, SMBType, configs[0].Type)
		assert.Equal(t, "beginning", configs[0].TailingMode)
		assert.Equal(t, expected, configs[0].SMB)
	})

	t.Run("integer poll_interval and defaults", func(t *testing.T) {
		configs, err := ParseYAML([]byte(`
logs:
  - type: smb
    path: "*.log"
    smb:
      host: samba
      share: logs
      username: datadog
      password: plain-text
      poll_interval: 2
`))
		require.NoError(t, err)
		require.Len(t, configs, 1)
		smb := configs[0].SMB
		require.NotNil(t, smb)
		assert.InDelta(t, 2.0, smb.PollInterval, 0)
		assert.Zero(t, smb.Port)
		assert.Empty(t, smb.Domain)
		assert.Equal(t, "plain-text", smb.Password)
		assert.NoError(t, configs[0].Validate())
	})

	t.Run("security settings", func(t *testing.T) {
		configs, err := ParseYAML([]byte(`
logs:
  - type: smb
    path: "app/*.log"
    smb:
      host: myacct.file.core.windows.net
      share: logs
      username: myacct
      password: "ENC[smb_account_key]"
      domain: CORP
      port: 1445
      poll_interval: 0.5
      require_encryption: true
`))
		require.NoError(t, err)
		require.Len(t, configs, 1)
		assert.Equal(t, &expectedStrict, configs[0].SMB)

		configs, err = ParseJSON([]byte(`[{"type":"smb","path":"app/*.log","smb":{"host":"old-nas.example.com","share":"logs","username":"u","password":"p","allow_smb2":true}}]`))
		require.NoError(t, err)
		assert.True(t, configs[0].SMB.AllowSMB2)
		assert.False(t, configs[0].SMB.RequireEncryption)
		assert.NoError(t, configs[0].Validate())

		// Both are off unless set.
		configs, err = ParseYAML([]byte("logs:\n  - type: smb\n    path: a.log\n    smb:\n      host: h.example.com\n      share: s\n      username: u\n"))
		require.NoError(t, err)
		assert.False(t, configs[0].SMB.AllowSMB2)
		assert.False(t, configs[0].SMB.RequireEncryption)
	})

	t.Run("no smb block", func(t *testing.T) {
		configs, err := ParseYAML([]byte("logs:\n  - type: file\n    path: /var/log/a.log\n"))
		require.NoError(t, err)
		assert.Nil(t, configs[0].SMB)
	})
}

func TestSMBConfigRedaction(t *testing.T) {
	cfg := validSMBLogsConfig()
	cfg.TailingMode = "beginning"
	cfg.SMB.Domain = "CORP"
	cfg.SMB.Port = 1445

	t.Run("Dump", func(t *testing.T) {
		for _, multiline := range []bool{true, false} {
			dump := cfg.Dump(multiline)
			assert.NotContains(t, dump, testSMBPassword)
			assert.Contains(t, dump, `Path: "app/*.log",`)
			assert.Contains(t, dump, `TailingMode: "beginning",`)
			assert.Contains(t, dump, `Host: "myacct.file.core.windows.net"`)
			assert.Contains(t, dump, `Password: "********"`)
		}
	})

	t.Run("PublicJSON", func(t *testing.T) {
		ret, err := cfg.PublicJSON()
		require.NoError(t, err)
		expectedJSON := `{"type":"smb","path":"app/*.log","start_position":"beginning",` +
			`"smb":{"host":"myacct.file.core.windows.net","share":"logs","username":"myacct","password":"********","domain":"CORP","port":1445},` +
			`"service":"demo-app","source":"demo"}`
		assert.Equal(t, expectedJSON, string(ret))
	})

	t.Run("PublicJSON without password", func(t *testing.T) {
		noPassword := validSMBLogsConfig()
		noPassword.SMB.Password = ""
		ret, err := noPassword.PublicJSON()
		require.NoError(t, err)
		assert.NotContains(t, string(ret), "password")
		assert.NotContains(t, string(ret), redactedSecret)
	})

	t.Run("fmt verbs", func(t *testing.T) {
		// The parser prints parsed configs with %#v and %+v; also cover verbs that would print
		// raw struct fields without a Formatter.
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
			for _, arg := range []interface{}{cfg, cfg.SMB, *cfg.SMB, []*SMBConfig{cfg.SMB}} {
				out := fmt.Sprintf(verb, arg)
				assert.NotContains(t, out, testSMBPassword, "verb %s on %T: %s", verb, arg, out)
			}
		}
		assert.Contains(t, fmt.Sprintf("%#v", cfg.SMB), `config.SMBConfig{Host: "myacct.file.core.windows.net"`)
		assert.Contains(t, fmt.Sprintf("%+v", cfg.SMB), `Password: "********"`)
		assert.Contains(t, fmt.Sprint(cfg.SMB), `Share: "logs"`)
		assert.Equal(t, "<nil>", fmt.Sprint((*SMBConfig)(nil)))
	})

	t.Run("marshal", func(t *testing.T) {
		// LogsConfig itself cannot be marshalled (it has a chan field), so embed the block the
		// way a caller serializing it would.
		wrapper := struct {
			SMB *SMBConfig `json:"smb" yaml:"smb"`
		}{SMB: cfg.SMB}

		j, err := json.Marshal(wrapper)
		require.NoError(t, err)
		assert.NotContains(t, string(j), testSMBPassword)
		assert.Contains(t, string(j), `"password":"********"`)

		y, err := yaml.Marshal(wrapper)
		require.NoError(t, err)
		assert.NotContains(t, string(y), testSMBPassword)
		assert.Contains(t, string(y), "host: myacct.file.core.windows.net")
		assert.Contains(t, string(y), "********")

		// redaction never mutates the config the launcher reads the credential from
		assert.Equal(t, testSMBPassword, cfg.SMB.Password)
	})
}

// The agent configcheck command and the flare config-check scrub logs configs with the default
// YAML scrubber; the flare copy of conf.d uses an ENC[]-preserving one. Both only know credential
// key names, so this guards the choice of "password" as the key name.
func TestSMBPasswordKeyIsScrubbed(t *testing.T) {
	yamlConfig := []byte(`logs:
  - type: smb
    path: app/*.log
    smb:
      host: myacct.file.core.windows.net
      share: logs
      username: myacct
      password: "` + testSMBPassword + `"
  - type: smb
    path: app/*.log
    smb:
      host: myacct.file.core.windows.net
      share: logs
      username: myacct
      password: "ENC[smb_account_key]"
`)
	// configcheck (resolved configs): both the plaintext password and the handle are masked
	scrubbed, err := scrubber.ScrubYaml(yamlConfig)
	require.NoError(t, err)
	assert.NotContains(t, string(scrubbed), testSMBPassword)
	assert.Contains(t, string(scrubbed), "myacct.file.core.windows.net")

	// flare copy of conf.d: the plaintext password is masked, the ENC[] handle is kept
	encAware := scrubber.New()
	scrubber.AddDefaultReplacers(encAware)
	encAware.SetPreserveENC(true)
	scrubbed, err = encAware.ScrubYaml(yamlConfig)
	require.NoError(t, err)
	assert.NotContains(t, string(scrubbed), testSMBPassword)
	assert.Contains(t, string(scrubbed), "ENC[smb_account_key]")

	// line scrubber, used for log lines and non-YAML flare files
	scrubbed, err = scrubber.ScrubBytes(yamlConfig)
	require.NoError(t, err)
	assert.NotContains(t, string(scrubbed), testSMBPassword)

	jsonConfig := []byte(`[{"type":"smb","path":"app/*.log","smb":{"host":"myacct.file.core.windows.net","share":"logs","username":"myacct","password":"` + testSMBPassword + `"}}]`)
	scrubbed, err = scrubber.ScrubJSON(jsonConfig)
	require.NoError(t, err)
	assert.NotContains(t, string(scrubbed), testSMBPassword)

	assert.NotContains(t, scrubJSONForLog(jsonConfig), testSMBPassword)
	assert.Contains(t, scrubJSONForLog(jsonConfig), `"host":"myacct.file.core.windows.net"`)
	assert.Contains(t, scrubJSONForLog([]byte("not json, password: "+testSMBPassword)), "not json")
	assert.NotContains(t, scrubJSONForLog([]byte("not json, password: "+testSMBPassword)), testSMBPassword)
}

func TestParserDebugLogsDoNotLeakSMBPassword(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(l, "debug")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "info") })

	_, err = ParseJSON([]byte(`[{"type":"smb","path":"app/*.log","smb":{"host":"myacct.file.core.windows.net","share":"logs","username":"myacct","password":"` + testSMBPassword + `"}}]`))
	require.NoError(t, err)
	_, err = ParseYAML([]byte("logs:\n  - type: smb\n    path: app/*.log\n    smb:\n      host: myacct.file.core.windows.net\n      share: logs\n      username: myacct\n      password: \"" + testSMBPassword + "\"\n"))
	require.NoError(t, err)
	_, err = ParseJSONOrYAML([]byte("logs:\n  - type: smb\n    smb:\n      password: " + testSMBPassword + "\n"))
	require.NoError(t, err)

	log.Flush()
	require.NoError(t, w.Flush())
	out := b.String()
	assert.Contains(t, out, "Parsing JSON logs config")
	assert.Contains(t, out, "Parsed YAML logs config")
	assert.Contains(t, out, "myacct.file.core.windows.net")
	assert.NotContains(t, out, testSMBPassword)
}

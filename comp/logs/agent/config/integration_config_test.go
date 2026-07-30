// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package config

import (
	"crypto/tls"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func TestValidateShouldSucceedWithValidConfigs(t *testing.T) {
	validConfigs := []*LogsConfig{
		{Type: FileType, Path: "/var/log/foo.log", FingerprintConfig: &types.FingerprintConfig{MaxBytes: 256, Count: 1, CountToSkip: 0, FingerprintStrategy: "line_checksum"}},
		{Type: TCPType, Port: 1234, FingerprintConfig: &types.FingerprintConfig{MaxBytes: 256, Count: 1, CountToSkip: 0, FingerprintStrategy: "line_checksum"}},
		{Type: UDPType, Port: 5678, FingerprintConfig: &types.FingerprintConfig{MaxBytes: 256, Count: 1, CountToSkip: 0, FingerprintStrategy: "line_checksum"}},
		{Type: TCPType, Port: 6514, TLS: &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key"}, FingerprintConfig: &types.FingerprintConfig{MaxBytes: 256, Count: 1, CountToSkip: 0, FingerprintStrategy: "line_checksum"}},
		{Type: DockerType, FingerprintConfig: &types.FingerprintConfig{MaxBytes: 256, Count: 1, CountToSkip: 0, FingerprintStrategy: "line_checksum"}},
		{Type: JournaldType, ProcessingRules: []*ProcessingRule{{Name: "foo", Type: ExcludeAtMatch, Pattern: ".*"}}, FingerprintConfig: &types.FingerprintConfig{MaxBytes: 256, Count: 1, CountToSkip: 0, FingerprintStrategy: "line_checksum"}},
	}

	for _, config := range validConfigs {
		err := config.Validate()
		assert.Nil(t, err)
	}
}

func TestValidateShouldFailWithInvalidConfigs(t *testing.T) {
	invalidConfigs := []*LogsConfig{
		{},
		{Type: FileType},
		{Type: TCPType},
		{Type: UDPType},
		{Type: TCPType, Port: 6514, TLS: &TLSListenerConfig{CertFile: "/cert"}},
		{Type: UDPType, Port: 514, TLS: &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key"}},
		{Type: TCPType, Port: 6514, TLS: &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", ClientAuth: "bogus"}},
		{Type: TCPType, Port: 6514, AllowedIPs: StringSliceField{"not-an-ip"}},
		{Type: TCPType, Port: 6514, TLS: &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", MinTLSVersion: "tls1.2"}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo", Type: "bar"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo", Type: ExcludeAtMatch}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Name: "foo", Pattern: ".*"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Type: ExcludeAtMatch, Pattern: ".*"}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Type: ExcludeAtMatch}}},
		{Type: DockerType, ProcessingRules: []*ProcessingRule{{Pattern: ".*"}}},
	}

	for _, config := range invalidConfigs {
		err := config.Validate()
		assert.NotNil(t, err)
	}
}

func TestAutoMultilineEnabled(t *testing.T) {
	decode := func(cfg string) *LogsConfig {
		lc := LogsConfig{}
		json.Unmarshal([]byte(cfg), &lc)
		return &lc
	}

	mockConfig := config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection", false)
	assert.False(t, decode(`{"auto_multi_line_detection":false}`).AutoMultiLineEnabled(mockConfig))

	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
	assert.False(t, decode(`{"auto_multi_line_detection":false}`).AutoMultiLineEnabled(mockConfig))

	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
	assert.True(t, decode(`{}`).AutoMultiLineEnabled(mockConfig))

	mockConfig.SetInTest("logs_config.auto_multi_line_detection", false)
	assert.True(t, decode(`{"auto_multi_line_detection":true}`).AutoMultiLineEnabled(mockConfig))

	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
	assert.True(t, decode(`{"auto_multi_line_detection":true}`).AutoMultiLineEnabled(mockConfig))

	mockConfig.SetInTest("logs_config.auto_multi_line_detection", false)
	assert.False(t, decode(`{}`).AutoMultiLineEnabled(mockConfig))

}

func TestExperimentalAdaptiveSamplingOptionsDecode(t *testing.T) {
	cfg := decode(`{"experimental_adaptive_sampling":{"enabled":true,"max_patterns":42,"rate_limit":2.5,"burst_size":17.5,"match_threshold":0.75,"tokenizer_max_input_bytes":512,"protect_important_logs":false,"tag_pattern_hash":true,"include":[{"regex":"foo.*bar"},{"sample":"my 123 fun log sample"}],"exclude":[{"regex":"baz.*qux"},{"sample":"my 456 bad log sample"}]}}`)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.Enabled)
	assert.True(t, *cfg.ExperimentalAdaptiveSampling.Enabled)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.MaxPatterns)
	assert.Equal(t, 42, *cfg.ExperimentalAdaptiveSampling.MaxPatterns)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.RateLimit)
	assert.Equal(t, 2.5, *cfg.ExperimentalAdaptiveSampling.RateLimit)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.BurstSize)
	assert.Equal(t, 17.5, *cfg.ExperimentalAdaptiveSampling.BurstSize)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.MatchThreshold)
	assert.Equal(t, 0.75, *cfg.ExperimentalAdaptiveSampling.MatchThreshold)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.TokenizerMaxInputBytes)
	assert.Equal(t, 512, *cfg.ExperimentalAdaptiveSampling.TokenizerMaxInputBytes)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.ProtectImportantLogs)
	assert.False(t, *cfg.ExperimentalAdaptiveSampling.ProtectImportantLogs)
	require.NotNil(t, cfg.ExperimentalAdaptiveSampling.TagPatternHash)
	assert.True(t, *cfg.ExperimentalAdaptiveSampling.TagPatternHash)
	require.Len(t, cfg.ExperimentalAdaptiveSampling.Include, 2)
	assert.Equal(t, "foo.*bar", cfg.ExperimentalAdaptiveSampling.Include[0].Regex)
	assert.Equal(t, "my 123 fun log sample", cfg.ExperimentalAdaptiveSampling.Include[1].Sample)
	require.Len(t, cfg.ExperimentalAdaptiveSampling.Exclude, 2)
	assert.Equal(t, "baz.*qux", cfg.ExperimentalAdaptiveSampling.Exclude[0].Regex)
	assert.Equal(t, "my 456 bad log sample", cfg.ExperimentalAdaptiveSampling.Exclude[1].Sample)
}

func TestExperimentalNoisyLogDetectionDecode(t *testing.T) {
	cfg := decode(`{"experimental_noisy_log_detection":true}`)
	require.NotNil(t, cfg.ExperimentalNoisyLogDetection)
	assert.True(t, *cfg.ExperimentalNoisyLogDetection)
}

func TestAutoMultiLineStatus(t *testing.T) {
	t.Run("per-source false overrides global true", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
		enabled, isDefault := decode(`{"auto_multi_line_detection":false}`).AutoMultiLineStatus(mockConfig)
		assert.False(t, enabled)
		assert.False(t, isDefault)
	})

	t.Run("per-source true overrides global false", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		mockConfig.SetInTest("logs_config.auto_multi_line_detection", false)
		enabled, isDefault := decode(`{"auto_multi_line_detection":true}`).AutoMultiLineStatus(mockConfig)
		assert.True(t, enabled)
		assert.False(t, isDefault)
	})

	t.Run("global explicitly true is not default", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
		enabled, isDefault := decode(`{}`).AutoMultiLineStatus(mockConfig)
		assert.True(t, enabled)
		assert.False(t, isDefault)
	})

	t.Run("global explicitly false is not default", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		mockConfig.SetInTest("logs_config.auto_multi_line_detection", false)
		enabled, isDefault := decode(`{}`).AutoMultiLineStatus(mockConfig)
		assert.False(t, enabled)
		assert.False(t, isDefault)
	})

	t.Run("autoMultiLine is configured by default", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		enabled, isDefault := decode(`{}`).AutoMultiLineStatus(mockConfig)
		assert.True(t, enabled)
		assert.True(t, isDefault)
	})

	t.Run("deprecated experimental true is not default", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		mockConfig.SetInTest("logs_config.experimental_auto_multi_line_detection", true)
		enabled, isDefault := decode(`{}`).AutoMultiLineStatus(mockConfig)
		assert.True(t, enabled)
		assert.False(t, isDefault)
	})

	t.Run("deprecated experimental false with auto true is not default", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		mockConfig.SetInTest("logs_config.experimental_auto_multi_line_detection", false)
		mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
		enabled, isDefault := decode(`{}`).AutoMultiLineStatus(mockConfig)
		assert.True(t, enabled)
		assert.False(t, isDefault)
	})
}

func decode(cfg string) *LogsConfig {
	lc := LogsConfig{}
	json.Unmarshal([]byte(cfg), &lc)
	return &lc
}

func TestLegacyAutoMultilineEnabled(t *testing.T) {
	mockConfig := config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection", false)
	assert.False(t, decode(`{"auto_multi_line_detection":false}`).LegacyAutoMultiLineEnabled(mockConfig))
	assert.False(t, decode(`{"auto_multi_line_detection":true}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
	assert.False(t, decode(`{"auto_multi_line_detection":false}`).LegacyAutoMultiLineEnabled(mockConfig))
	assert.False(t, decode(`{"auto_multi_line_detection":true}`).LegacyAutoMultiLineEnabled(mockConfig))

	assert.True(t, decode(`{"auto_multi_line_sample_size": 2}`).LegacyAutoMultiLineEnabled(mockConfig))
	assert.True(t, decode(`{"auto_multi_line_match_threshold": 0.4}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.force_auto_multi_line_detection_v1", true)
	assert.True(t, decode(`{}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
	mockConfig.SetInTest("logs_config.auto_multi_line_default_sample_size", 10)
	assert.True(t, decode(`{}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
	mockConfig.SetInTest("logs_config.auto_multi_line_default_match_timeout", 100)
	assert.True(t, decode(`{}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_detection", true)
	mockConfig.SetInTest("logs_config.auto_multi_line_default_match_threshold", 501)
	assert.True(t, decode(`{}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.force_auto_multi_line_detection_v1", true)
	assert.False(t, decode(`{"auto_multi_line_detection":false}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_default_sample_size", 10)
	assert.True(t, decode(`{"auto_multi_line_detection":true}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_default_match_timeout", 100)
	assert.True(t, decode(`{"auto_multi_line_detection":true}`).LegacyAutoMultiLineEnabled(mockConfig))

	mockConfig = config.NewMock(t)
	mockConfig.SetInTest("logs_config.auto_multi_line_default_match_threshold", 501)
	assert.True(t, decode(`{"auto_multi_line_detection":true}`).LegacyAutoMultiLineEnabled(mockConfig))
}

func TestEncoding(t *testing.T) {
	assert.Equal(t, UTF16BE, decode(`{"encoding":"utf-16-be"}`).Encoding)
	assert.Equal(t, UTF16LE, decode(`{"encoding":"utf-16-le"}`).Encoding)
	assert.Equal(t, SHIFTJIS, decode(`{"encoding":"shift-jis"}`).Encoding)
	assert.Equal(t, "", decode(`{}`).Encoding)
}

func TestConfigDump(t *testing.T) {
	config := LogsConfig{Type: FileType, Path: "/var/log/foo.log"}
	dump := config.Dump(true)
	assert.Contains(t, dump, `Path: "/var/log/foo.log",`)
}

func TestPublicJSON(t *testing.T) {
	config := LogsConfig{
		Type:     FileType,
		Path:     "/var/log/foo.log",
		Encoding: "utf-8",
		Service:  "foo",
		Tags:     []string{"foo:bar"},
		Source:   "bar",
	}
	ret, err := config.PublicJSON()
	assert.NoError(t, err)

	expectedJSON := `{"type":"file","path":"/var/log/foo.log","encoding":"utf-8","service":"foo","source":"bar","tags":["foo:bar"]}`
	assert.Equal(t, expectedJSON, string(ret))
}

func TestFingerprintConfig(t *testing.T) {
	validConfigs := []*types.FingerprintConfig{
		{Count: 30, CountToSkip: 0, FingerprintStrategy: "byte_checksum"},
		{MaxBytes: 1024, Count: 10, CountToSkip: 2, FingerprintStrategy: "line_checksum"},
		{Count: 50, CountToSkip: 0, FingerprintStrategy: "byte_checksum"},
	}

	for _, config := range validConfigs {
		err := ValidateFingerprintConfig(config)
		assert.Nil(t, err)
	}

	invalidConfigs := []*types.FingerprintConfig{
		{MaxBytes: 0, Count: 0, CountToSkip: 0},
		{MaxBytes: -1, Count: 0, CountToSkip: 0},
		{MaxBytes: 256, Count: -1, CountToSkip: 0},
		{MaxBytes: 256, Count: 0, CountToSkip: -1},
	}

	for _, config := range invalidConfigs {
		err := ValidateFingerprintConfig(config)
		assert.NotNil(t, err)
	}
}

func TestValidateTLSConfig(t *testing.T) {
	t.Run("valid TLS with cert and key", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/path/to/cert.pem", KeyFile: "/path/to/key.pem"},
		}
		err := cfg.validateTLS()
		assert.Nil(t, err)
	})

	t.Run("valid TLS with mutual auth", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", CAFile: "/ca", ClientAuth: "required"},
		}
		err := cfg.validateTLS()
		assert.Nil(t, err)
	})

	t.Run("nil TLS is valid", func(t *testing.T) {
		cfg := &LogsConfig{Type: TCPType, Port: 1234}
		err := cfg.validateTLS()
		assert.Nil(t, err)
	})

	t.Run("TLS on non-TCP type fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: UDPType,
			Port: 514,
			TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key"},
		}
		err := cfg.validateTLS()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "only supported for tcp")
	})

	t.Run("TLS missing key_file fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/cert"},
		}
		err := cfg.validateTLS()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "cert_file and key_file")
	})

	t.Run("TLS missing cert_file fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{KeyFile: "/key"},
		}
		err := cfg.validateTLS()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "cert_file and key_file")
	})

	t.Run("optional client_auth without ca_file fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", ClientAuth: "optional"},
		}
		err := cfg.validateTLS()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "ca_file")
	})

	t.Run("required client_auth without ca_file fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", ClientAuth: "required"},
		}
		err := cfg.validateTLS()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "ca_file")
	})

	t.Run("optional client_auth with ca_file is OK", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", CAFile: "/ca", ClientAuth: "optional"},
		}
		err := cfg.validateTLS()
		assert.Nil(t, err)
	})

	t.Run("unrecognized client_auth fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", ClientAuth: "verify_client"},
		}
		err := cfg.validateTLS()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "unrecognized client_auth")
	})

	t.Run("unrecognized min_tls_version fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
			TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", MinTLSVersion: "tls1.2"},
		}
		err := cfg.validateTLS()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "unrecognized min_tls_version")
	})

	t.Run("all valid client_auth values pass", func(t *testing.T) {
		for _, auth := range []string{"", "none", "optional", "required"} {
			cfg := &LogsConfig{
				Type: TCPType,
				Port: 6514,
				TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", CAFile: "/ca", ClientAuth: auth},
			}
			err := cfg.validateTLS()
			assert.Nil(t, err, "client_auth %q should be valid", auth)
		}
	})

	t.Run("old client_auth values are rejected", func(t *testing.T) {
		for _, auth := range []string{"request", "require", "verify", "require_and_verify"} {
			cfg := &LogsConfig{
				Type: TCPType,
				Port: 6514,
				TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", ClientAuth: auth},
			}
			err := cfg.validateTLS()
			assert.NotNil(t, err, "client_auth %q should be rejected", auth)
			assert.Contains(t, err.Error(), "unrecognized client_auth")
		}
	})

	t.Run("all valid min_tls_version values pass", func(t *testing.T) {
		for _, v := range []string{"", "tlsv1.2", "tlsv1.3"} {
			cfg := &LogsConfig{
				Type: TCPType,
				Port: 6514,
				TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", MinTLSVersion: v},
			}
			err := cfg.validateTLS()
			assert.Nil(t, err, "min_tls_version %q should be valid", v)
		}
	})

	t.Run("deprecated TLS versions are rejected", func(t *testing.T) {
		for _, v := range []string{"tlsv1.0", "tlsv1.1"} {
			cfg := &LogsConfig{
				Type: TCPType,
				Port: 6514,
				TLS:  &TLSListenerConfig{CertFile: "/cert", KeyFile: "/key", MinTLSVersion: v},
			}
			err := cfg.validateTLS()
			assert.NotNil(t, err, "min_tls_version %q should be rejected", v)
			assert.Contains(t, err.Error(), "unrecognized min_tls_version")
		}
	})
}

func TestParseTLSVersion(t *testing.T) {
	v, err := parseTLSVersion("tlsv1.2")
	require.NoError(t, err)
	assert.Equal(t, uint16(0x0303), v)

	v, err = parseTLSVersion("tlsv1.3")
	require.NoError(t, err)
	assert.Equal(t, uint16(0x0304), v)

	v, err = parseTLSVersion("")
	require.NoError(t, err)
	assert.Equal(t, uint16(0x0303), v)

	v, err = parseTLSVersion("TLSv1.3")
	require.NoError(t, err)
	assert.Equal(t, uint16(0x0304), v)

	_, err = parseTLSVersion("invalid")
	assert.Error(t, err)

	_, err = parseTLSVersion("tlsv1.0")
	assert.Error(t, err, "TLS 1.0 should no longer be accepted")

	_, err = parseTLSVersion("tlsv1.1")
	assert.Error(t, err, "TLS 1.1 should no longer be accepted")
}

func TestParseClientAuth(t *testing.T) {
	v, err := parseClientAuth("")
	require.NoError(t, err)
	assert.Equal(t, tls.NoClientCert, v)

	v, err = parseClientAuth("none")
	require.NoError(t, err)
	assert.Equal(t, tls.NoClientCert, v)

	v, err = parseClientAuth("optional")
	require.NoError(t, err)
	assert.Equal(t, tls.VerifyClientCertIfGiven, v)

	v, err = parseClientAuth("required")
	require.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, v)

	_, err = parseClientAuth("bogus")
	assert.Error(t, err)
}

func TestValidateWildcardWithBeginningMode(t *testing.T) {
	validConfigs := []*LogsConfig{
		{Type: FileType, Path: "/var/log/*.log", TailingMode: "beginning"},
		{Type: FileType, Path: "/var/log/app-?.log", TailingMode: "beginning"},
		{Type: FileType, Path: "/var/log/[abc].log", TailingMode: "beginning"},
		{Type: FileType, Path: "/var/log/**/*.log", TailingMode: "forceBeginning"},
		{Type: FileType, Path: "/tmp/test*.log", TailingMode: "forceBeginning"},
	}

	for _, config := range validConfigs {
		err := config.Validate()
		assert.Nil(t, err, "Wildcard path %s with tailing mode %s should be valid", config.Path, config.TailingMode)
	}
}

func TestValidateSyslogFormatWithEncoding(t *testing.T) {
	t.Run("syslog format with non-UTF8 encoding warns but passes", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:     FileType,
			Path:     "/var/log/syslog",
			Source:   "mysource",
			Format:   SyslogFormat,
			Encoding: UTF16LE,
		}
		err := cfg.Validate()
		assert.Nil(t, err)
	})

	t.Run("syslog format without encoding passes", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:   FileType,
			Path:   "/var/log/syslog",
			Format: SyslogFormat,
		}
		err := cfg.Validate()
		assert.Nil(t, err)
	})
}

func TestValidateIPFilter(t *testing.T) {
	t.Run("valid CIDR entries pass", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:       TCPType,
			Port:       6514,
			AllowedIPs: StringSliceField{"10.0.0.0/8", "192.168.1.0/24"},
			DeniedIPs:  StringSliceField{"10.0.0.99"},
		}
		err := cfg.validateIPFilter()
		assert.Nil(t, err)
	})

	t.Run("valid single IPs pass", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:       UDPType,
			Port:       514,
			AllowedIPs: StringSliceField{"10.0.0.1", "::1"},
		}
		err := cfg.validateIPFilter()
		assert.Nil(t, err)
	})

	t.Run("invalid allowed IP fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:       TCPType,
			Port:       6514,
			AllowedIPs: StringSliceField{"not-an-ip"},
		}
		err := cfg.validateIPFilter()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "allowed_ips")
	})

	t.Run("invalid denied IP fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:      TCPType,
			Port:      6514,
			DeniedIPs: StringSliceField{"999.999.999.999"},
		}
		err := cfg.validateIPFilter()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "denied_ips")
	})

	t.Run("IP filter on file type fails", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:       FileType,
			Path:       "/var/log/test.log",
			AllowedIPs: StringSliceField{"10.0.0.1"},
		}
		err := cfg.validateIPFilter()
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "only supported for")
	})

	t.Run("empty lists pass", func(t *testing.T) {
		cfg := &LogsConfig{
			Type: TCPType,
			Port: 6514,
		}
		err := cfg.validateIPFilter()
		assert.Nil(t, err)
	})

	t.Run("both allowed and denied coexist", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:       TCPType,
			Port:       6514,
			AllowedIPs: StringSliceField{"10.0.0.0/8"},
			DeniedIPs:  StringSliceField{"10.0.0.0/24"},
		}
		err := cfg.validateIPFilter()
		assert.Nil(t, err)
	})

	t.Run("IPv6 CIDR passes", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:       TCPType,
			Port:       6514,
			AllowedIPs: StringSliceField{"fd00::/64"},
		}
		err := cfg.validateIPFilter()
		assert.Nil(t, err)
	})

	t.Run("UDP type passes", func(t *testing.T) {
		cfg := &LogsConfig{
			Type:      UDPType,
			Port:      514,
			DeniedIPs: StringSliceField{"10.0.0.99"},
		}
		err := cfg.validateIPFilter()
		assert.Nil(t, err)
	})
}

func TestIsAttributeParsingEnabled(t *testing.T) {
	remapRule := &ProcessingRule{
		Type: RemapSource,
		Name: "remap",
		Matching: []*SourceMatchEntry{
			{Attribute: "syslog.appname", Value: "nginx", NewSource: "nginx"},
		},
	}
	otherRule := &ProcessingRule{Type: ExcludeAtMatch, Name: "ex", Pattern: ".*"}

	t.Run("explicit true wins", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		enabled := true
		c := &LogsConfig{AttributeParsing: &enabled}
		assert.True(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	t.Run("explicit false overrides remap rule", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		disabled := false
		c := &LogsConfig{AttributeParsing: &disabled, ProcessingRules: []*ProcessingRule{remapRule}}
		assert.False(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	t.Run("nil auto-enables with per-source remap_source", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		c := &LogsConfig{ProcessingRules: []*ProcessingRule{remapRule}}
		assert.True(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	t.Run("nil with non-remap per-source rule stays off", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		c := &LogsConfig{ProcessingRules: []*ProcessingRule{otherRule}}
		assert.False(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	t.Run("nil auto-enables with global remap_source", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		mockConfig.SetInTest("logs_config.processing_rules", []map[string]any{
			{
				"type": "remap_source",
				"name": "global-remap",
				"matching": []map[string]any{
					{"attribute": "syslog.appname", "value": "nginx", "new_source": "nginx"},
				},
			},
		})
		c := &LogsConfig{}
		assert.True(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	t.Run("nil with no rules defaults off", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		c := &LogsConfig{}
		assert.False(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	// debug_attr_parsing has no effect unless the syslog parser runs, so it must
	// imply attribute parsing when attribute_parsing is left unset. Otherwise the
	// decoder installs the noop parser and the structured envelope is never
	// rendered.
	t.Run("nil auto-enables with debug_attr_parsing", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		debug := true
		c := &LogsConfig{DebugAttrParsing: &debug}
		assert.True(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	t.Run("explicit false overrides debug_attr_parsing", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		disabled := false
		debug := true
		c := &LogsConfig{AttributeParsing: &disabled, DebugAttrParsing: &debug}
		assert.False(t, c.IsAttributeParsingEnabled(mockConfig))
	})

	t.Run("nil with debug_attr_parsing off stays off", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		debug := false
		c := &LogsConfig{DebugAttrParsing: &debug}
		assert.False(t, c.IsAttributeParsingEnabled(mockConfig))
	})
}

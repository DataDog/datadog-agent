// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	expectedDefaultConfig := &Config{
		rules:          []byte(staticRecommendedRule),
		wafTimeout:     defaultWAFTimeout,
		traceRateLimit: defaultTraceRate,
		obfuscator: ObfuscatorConfig{
			KeyRegex:   defaultObfuscatorKeyRegex,
			ValueRegex: defaultObfuscatorValueRegex,
		},
	}

	t.Run("default", func(t *testing.T) {
		restoreEnv := cleanEnv()
		defer restoreEnv()
		cfg, err := newConfig()
		require.NoError(t, err)
		require.Equal(t, expectedDefaultConfig, cfg)
	})

	t.Run("waf-timeout", func(t *testing.T) {
		t.Run("parsable", func(t *testing.T) {
			expCfg := *expectedDefaultConfig
			expCfg.wafTimeout = 5 * time.Second
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "5s"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})

		t.Run("parsable-default-microsecond", func(t *testing.T) {
			expCfg := *expectedDefaultConfig
			expCfg.wafTimeout = 1 * time.Microsecond
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "1"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})

		t.Run("not-parsable", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "not a duration string"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("negative", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "-1s"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("zero", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "0"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("empty-string", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, ""))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})
	})

	t.Run("rules", func(t *testing.T) {
		t.Run("empty-string", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			os.Setenv(rulesEnvVar, "")
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("file-not-found", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			os.Setenv(rulesEnvVar, "i do not exist")
			cfg, err := newConfig()
			require.Error(t, err)
			require.Nil(t, cfg)
		})

		t.Run("local-file", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			file, err := os.CreateTemp("", "example-*")
			require.NoError(t, err)
			defer func() {
				file.Close()
				os.Remove(file.Name())
			}()
			expectedRules := `custom rule file content`
			expCfg := *expectedDefaultConfig
			expCfg.rules = []byte(expectedRules)
			_, err = file.WriteString(expectedRules)
			require.NoError(t, err)
			os.Setenv(rulesEnvVar, file.Name())
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})
	})

	t.Run("trace-rate-limit", func(t *testing.T) {
		t.Run("parsable", func(t *testing.T) {
			expCfg := *expectedDefaultConfig
			expCfg.traceRateLimit = 1234567890
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(traceRateLimitEnvVar, "1234567890"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})

		t.Run("not-parsable", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "not a uint"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("negative", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "-1"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("zero", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "0"))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("empty-string", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, ""))
			cfg, err := newConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})
	})

	t.Run("obfuscator", func(t *testing.T) {
		t.Run("key-regexp", func(t *testing.T) {
			t.Run("env-var-normal", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.obfuscator.KeyRegex = "test"
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorKeyEnvVar, "test"))
				cfg, err := newConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("env-var-empty", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.obfuscator.KeyRegex = ""
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorKeyEnvVar, ""))
				cfg, err := newConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("compile-error", func(t *testing.T) {
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorKeyEnvVar, "+"))
				cfg, err := newConfig()
				require.NoError(t, err)
				require.Equal(t, expectedDefaultConfig, cfg)
			})
		})

		t.Run("value-regexp", func(t *testing.T) {
			t.Run("env-var-normal", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.obfuscator.ValueRegex = "test"
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorValueEnvVar, "test"))
				cfg, err := newConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("env-var-empty", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.obfuscator.ValueRegex = ""
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorValueEnvVar, ""))
				cfg, err := newConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("compile-error", func(t *testing.T) {
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorValueEnvVar, "+"))
				cfg, err := newConfig()
				require.NoError(t, err)
				require.Equal(t, expectedDefaultConfig, cfg)
			})
		})
	})
}

func cleanEnv() func() {
	env := map[string]string{
		wafTimeoutEnvVar:      os.Getenv(wafTimeoutEnvVar),
		rulesEnvVar:           os.Getenv(rulesEnvVar),
		traceRateLimitEnvVar:  os.Getenv(traceRateLimitEnvVar),
		obfuscatorKeyEnvVar:   os.Getenv(obfuscatorKeyEnvVar),
		obfuscatorValueEnvVar: os.Getenv(obfuscatorValueEnvVar),
	}
	for k := range env {
		if err := os.Unsetenv(k); err != nil {
			panic(err)
		}
	}
	return func() {
		for k, v := range env {
			restoreEnv(k, v)
		}
	}
}

func restoreEnv(key, value string) {
	if value != "" {
		if err := os.Setenv(key, value); err != nil {
			panic(err)
		}
	} else {
		if err := os.Unsetenv(key); err != nil {
			panic(err)
		}
	}
}

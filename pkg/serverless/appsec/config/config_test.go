// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/appsec-internal-go/appsec"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	expectedDefaultConfig := &Config{
		Rules:          []byte(appsec.StaticRecommendedRules),
		WafTimeout:     defaultWAFTimeout,
		TraceRateLimit: defaultTraceRate,
		Obfuscator: ObfuscatorConfig{
			KeyRegex:   defaultObfuscatorKeyRegex,
			ValueRegex: defaultObfuscatorValueRegex,
		},
	}

	t.Run("default", func(t *testing.T) {
		restoreEnv := cleanEnv()
		defer restoreEnv()
		cfg, err := NewConfig()
		require.NoError(t, err)
		require.Equal(t, expectedDefaultConfig, cfg)
	})

	t.Run("waf-timeout", func(t *testing.T) {
		t.Run("parsable", func(t *testing.T) {
			expCfg := *expectedDefaultConfig
			expCfg.WafTimeout = 5 * time.Second
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "5s"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})

		t.Run("parsable-default-microsecond", func(t *testing.T) {
			expCfg := *expectedDefaultConfig
			expCfg.WafTimeout = 1 * time.Microsecond
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "1"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})

		t.Run("not-parsable", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "not a duration string"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("negative", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "-1s"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("zero", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "0"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("empty-string", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, ""))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})
	})

	t.Run("rules", func(t *testing.T) {
		t.Run("empty-string", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			os.Setenv(rulesEnvVar, "")
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("file-not-found", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			os.Setenv(rulesEnvVar, "i do not exist")
			cfg, err := NewConfig()
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
			expCfg.Rules = []byte(expectedRules)
			_, err = file.WriteString(expectedRules)
			require.NoError(t, err)
			os.Setenv(rulesEnvVar, file.Name())
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})
	})

	t.Run("trace-rate-limit", func(t *testing.T) {
		t.Run("parsable", func(t *testing.T) {
			expCfg := *expectedDefaultConfig
			expCfg.TraceRateLimit = 1234567890
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(traceRateLimitEnvVar, "1234567890"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, &expCfg, cfg)
		})

		t.Run("not-parsable", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "not a uint"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("negative", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "-1"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("zero", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, "0"))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})

		t.Run("empty-string", func(t *testing.T) {
			restoreEnv := cleanEnv()
			defer restoreEnv()
			require.NoError(t, os.Setenv(wafTimeoutEnvVar, ""))
			cfg, err := NewConfig()
			require.NoError(t, err)
			require.Equal(t, expectedDefaultConfig, cfg)
		})
	})

	t.Run("obfuscator", func(t *testing.T) {
		t.Run("key-regexp", func(t *testing.T) {
			t.Run("env-var-normal", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.Obfuscator.KeyRegex = "test"
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorKeyEnvVar, "test"))
				cfg, err := NewConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("env-var-empty", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.Obfuscator.KeyRegex = ""
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorKeyEnvVar, ""))
				cfg, err := NewConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("compile-error", func(t *testing.T) {
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorKeyEnvVar, "+"))
				cfg, err := NewConfig()
				require.NoError(t, err)
				require.Equal(t, expectedDefaultConfig, cfg)
			})
		})

		t.Run("value-regexp", func(t *testing.T) {
			t.Run("env-var-normal", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.Obfuscator.ValueRegex = "test"
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorValueEnvVar, "test"))
				cfg, err := NewConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("env-var-empty", func(t *testing.T) {
				expCfg := *expectedDefaultConfig
				expCfg.Obfuscator.ValueRegex = ""
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorValueEnvVar, ""))
				cfg, err := NewConfig()
				require.NoError(t, err)
				require.Equal(t, &expCfg, cfg)
			})
			t.Run("compile-error", func(t *testing.T) {
				restoreEnv := cleanEnv()
				defer restoreEnv()
				require.NoError(t, os.Setenv(obfuscatorValueEnvVar, "+"))
				cfg, err := NewConfig()
				require.NoError(t, err)
				require.Equal(t, expectedDefaultConfig, cfg)
			})
		})
	})

	t.Run("standalone", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			env        string
			standalone bool
		}{
			{
				name: "unset",
			},
			{
				name:       "non-bool env",
				env:        "A5M",
				standalone: true,
			},
			{
				name: "env=true",
				env:  "true",
			},
			{
				name: "env=1",
				env:  "1",
			},
			{
				name:       "env=false",
				env:        "false",
				standalone: true,
			},
			{
				name:       "env=0",
				env:        "0",
				standalone: true,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				restoreEnv := cleanEnv()
				defer restoreEnv()
				// This is what happens at init() in config.go
				if tc.env != "" {
					require.NoError(t, os.Setenv(tracingEnabledEnvVar, tc.env))
				}
				standalone = isStandalone()
				require.Equal(t, tc.standalone, IsStandalone())
			})
		}

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

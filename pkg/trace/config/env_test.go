package config

import (
	"os"
	"testing"

	"github.com/StackVista/stackstate-agent/pkg/config"
	log "github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	log.UseLogger(log.Disabled)
	os.Exit(m.Run())
}

func TestLoadEnv(t *testing.T) {
	t.Run("overrides", func(t *testing.T) {
		// tests that newer envs. override deprecated ones
		for _, tt := range []struct {
			envOld, envNew, key string
		}{
			{"HTTPS_PROXY", "STS_PROXY_HTTPS", "proxy.https"},
			{"STS_CONNECTION_LIMIT", "STS_APM_CONNECTION_LIMIT", "apm_config.connection_limit"},
			{"STS_RECEIVER_PORT", "STS_APM_RECEIVER_PORT", "apm_config.receiver_port"},
			{"STS_MAX_EPS", "STS_MAX_EPS", "apm_config.max_events_per_second"},
			{"STS_MAX_TPS", "STS_APM_MAX_TPS", "apm_config.max_traces_per_second"},
			{"STS_IGNORE_RESOURCE", "STS_APM_IGNORE_RESOURCES", "apm_config.ignore_resources"},
		} {
			assert := assert.New(t)
			err := os.Setenv(tt.envOld, "1,2,3")
			assert.NoError(err)
			defer os.Unsetenv(tt.envOld)
			err = os.Setenv(tt.envNew, "4,5,6")
			assert.NoError(err)
			defer os.Unsetenv(tt.envNew)
			_, err = Load("./testdata/full.yaml")
			assert.NoError(err)
			if tt.envNew == "STS_APM_IGNORE_RESOURCES" {
				assert.Equal([]string{"4", "5", "6"}, config.Datadog.GetStringSlice(tt.key))
			} else {
				assert.Equal("4,5,6", config.Datadog.Get(tt.key))
			}
		}
	})

	for _, ext := range []string{"yaml", "ini"} {
		t.Run(ext, func(t *testing.T) {
			env := "STS_API_KEY"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "123")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal("123", cfg.Endpoints[0].APIKey)
			})

			env = "STS_SITE"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "my-site.com")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/undocumented." + ext)
				assert.NoError(err)
				assert.Equal(apiEndpointPrefix+"my-site.com", cfg.Endpoints[0].Host)
			})

			env = "STS_APM_ENABLED"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "true")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.True(cfg.Enabled)
			})

			env = "STS_APM_STS_URL"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "my-site.com")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal("my-site.com", cfg.Endpoints[0].Host)
			})

			env = "HTTPS_PROXY"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "my-proxy.url")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal("my-proxy.url", cfg.ProxyURL.String())
			})

			env = "STS_PROXY_HTTPS"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "my-proxy.url")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal("my-proxy.url", cfg.ProxyURL.String())
			})

			env = "STS_HOSTNAME"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "local.host")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal("local.host", cfg.Hostname)
			})

			env = "STS_BIND_HOST"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "bindhost.com")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal("bindhost.com", cfg.StatsdHost)
			})

			for _, envKey := range []string{
				"STS_RECEIVER_PORT",
				"STS_APM_RECEIVER_PORT",
			} {
				t.Run(envKey, func(t *testing.T) {
					assert := assert.New(t)
					err := os.Setenv(envKey, "1234")
					assert.NoError(err)
					defer os.Unsetenv(envKey)
					cfg, err := Load("./testdata/full." + ext)
					assert.NoError(err)
					assert.Equal(1234, cfg.ReceiverPort)
				})
			}

			env = "STS_DOGSTATSD_PORT"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "4321")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal(4321, cfg.StatsdPort)
			})

			env = "STS_APM_NON_LOCAL_TRAFFIC"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "true")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/undocumented." + ext)
				assert.NoError(err)
				assert.Equal("0.0.0.0", cfg.ReceiverHost)
			})

			for _, envKey := range []string{
				"STS_IGNORE_RESOURCE",
				"STS_APM_IGNORE_RESOURCES",
			} {
				t.Run(envKey, func(t *testing.T) {
					assert := assert.New(t)
					err := os.Setenv(envKey, "1,2,3")
					assert.NoError(err)
					defer os.Unsetenv(envKey)
					cfg, err := Load("./testdata/full." + ext)
					assert.NoError(err)
					assert.Equal([]string{"1", "2", "3"}, cfg.Ignore["resource"])
				})
			}

			env = "STS_LOG_LEVEL"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "warn")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal("warn", cfg.LogLevel)
			})

			env = "STS_APM_ANALYZED_SPANS"
			t.Run(env, func(t *testing.T) {
				assert := assert.New(t)
				err := os.Setenv(env, "web|http.request=1,db|sql.query=0.5")
				assert.NoError(err)
				defer os.Unsetenv(env)
				cfg, err := Load("./testdata/full." + ext)
				assert.NoError(err)
				assert.Equal(map[string]map[string]float64{
					"web": {"http.request": 1},
					"db":  {"sql.query": 0.5},
				}, cfg.AnalyzedSpansByService)
			})

			for _, envKey := range []string{
				"STS_CONNECTION_LIMIT",
				"STS_APM_CONNECTION_LIMIT",
			} {
				t.Run(envKey, func(t *testing.T) {
					assert := assert.New(t)
					err := os.Setenv(envKey, "50")
					assert.NoError(err)
					defer os.Unsetenv(envKey)
					cfg, err := Load("./testdata/full." + ext)
					assert.NoError(err)
					assert.Equal(50, cfg.ConnectionLimit)
				})
			}

			for _, envKey := range []string{
				"STS_MAX_TPS",
				"STS_APM_MAX_TPS",
			} {
				t.Run(envKey, func(t *testing.T) {
					assert := assert.New(t)
					err := os.Setenv(envKey, "6")
					assert.NoError(err)
					defer os.Unsetenv(envKey)
					cfg, err := Load("./testdata/full." + ext)
					assert.NoError(err)
					assert.Equal(6., cfg.MaxTPS)
				})
			}

			for _, envKey := range []string{
				"STS_MAX_EPS",
				"STS_APM_MAX_EPS",
			} {
				t.Run(envKey, func(t *testing.T) {
					assert := assert.New(t)
					err := os.Setenv(envKey, "7")
					assert.NoError(err)
					defer os.Unsetenv(envKey)
					cfg, err := Load("./testdata/full." + ext)
					assert.NoError(err)
					assert.Equal(7., cfg.MaxEPS)
				})
			}
		})
	}
}

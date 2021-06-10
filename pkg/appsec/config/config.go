package config

// AgentConfig represents the agent configuration API.
type AgentConfig interface {
	SetKnown(s string)
	BindEnvAndSetDefault(key string, v interface{}, env ...string)
	GetBool(key string) bool
	GetString(key string) string
}

// InitConfig initializes the config defaults on a config
func InitConfig(cfg AgentConfig) {
	cfg.SetKnown("appsec_config.enabled")
	cfg.SetKnown("appsec_config.api.http.listen_addr")
	cfg.SetKnown("appsec_config.backend.base_url")
	cfg.SetKnown("appsec_config.api_key")
	cfg.SetKnown("appsec_config.backend.proxy")

	cfg.BindEnvAndSetDefault("appsec_config.enabled", false, "DD_APPSEC_ENABLED")
	cfg.BindEnvAndSetDefault("appsec_config.api.http.listen_addr", "0.0.0.0:8127", "DD_APPSEC_API_HTTP_LISTEN_ADDR")
	cfg.BindEnvAndSetDefault("appsec_config.backend.base_url", "FIXME", "DD_APPSEC_BACKEND_BASE_URL") //nolint:errcheck
	cfg.BindEnvAndSetDefault("appsec_config.api_key", "", "DD_APPSEC_API_KEY")                        //nolint:errcheck
	cfg.BindEnvAndSetDefault("appsec_config.backend.proxy", "", "DD_APPSEC_BACKEND_API_PROXY")        //nolint:errcheck
}

// Config handles the interpretation of the configuration. It is also a simple
// structure to share across all the components, with 100% safe and reliable
// values.
type Config struct {
	Enabled bool

	// HTTP API
	HTTPAPIListenAddr string

	// Backend API
	BackendAPIBaseURL string
	BackendAPIKey     string
	BackendAPIProxy   string
}

func FromAgentConfig(cfg AgentConfig) *Config {
	return &Config{
		Enabled:           cfg.GetBool("appsec_config.enabled"),
		HTTPAPIListenAddr: cfg.GetString("appsec_config.api.http.listen_addr"),
		BackendAPIKey:     cfg.GetString("appsec_config.api_key"),
		BackendAPIBaseURL: cfg.GetString("appsec_config.backend.base_url"),
		BackendAPIProxy:   cfg.GetString("appsec_config.backend.proxy"),
	}
}

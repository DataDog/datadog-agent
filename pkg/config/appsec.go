package config

// Default configuration values
const (
	DefaultAppSecEnabled        = true
	DefaultAppSecDDUrl          = ""
	DefaultAppSecMaxPayloadSize = 5 * 1024 * 1024
)

// setupAppSec initializes the configuration values of the appsec agent.
func setupAppSec(cfg Config) {
	cfg.BindEnvAndSetDefault("appsec_config.enabled", DefaultAppSecEnabled, "DD_APPSEC_ENABLED")
	cfg.BindEnvAndSetDefault("appsec_config.appsec_dd_url", DefaultAppSecDDUrl, "DD_APPSEC_DD_URL")
	cfg.BindEnvAndSetDefault("appsec_config.max_payload_size", DefaultAppSecMaxPayloadSize, "DD_APPSEC_MAX_PAYLOAD_SIZE")
}

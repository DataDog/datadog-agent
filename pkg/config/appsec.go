package config

// setupAppSec initializes the configuration values of the appsec agent.
func setupAppSec(cfg Config) {
	cfg.BindEnvAndSetDefault("appsec_config.enabled", true, "DD_APPSEC_ENABLED")
	cfg.BindEnvAndSetDefault("appsec_config.appsec_dd_url", "", "DD_APPSEC_DD_URL")
}

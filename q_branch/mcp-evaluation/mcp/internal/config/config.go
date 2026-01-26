package config

import (
	"flag"
	"fmt"
	"os"
)

// Config holds the server configuration
type Config struct {
	Port     int
	LogLevel string
	Mode     string
}

// Load loads configuration from command-line flags, environment variables, and defaults
func Load() (*Config, error) {
	cfg := &Config{}

	// Define command-line flags
	flag.IntVar(&cfg.Port, "port", getEnvInt("MCP_EVAL_PORT", 8080), "Port to listen on")
	flag.StringVar(&cfg.LogLevel, "loglevel", getEnvString("MCP_EVAL_LOGLEVEL", "info"), "Log level (debug, info, warn, error)")
	flag.StringVar(&cfg.Mode, "mode", getEnvString("MCP_EVAL_MODE", "bash"), "Server mode: bash, safe-shell, tools, tools-safe-shell, tools-bash, tools-plus")

	flag.Parse()

	return cfg, cfg.validate()
}

// validate ensures the configuration is valid
func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be between 1 and 65535)", c.Port)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level: %s (must be one of: debug, info, warn, error)", c.LogLevel)
	}

	// Validate mode
	validModes := map[string]bool{
		"bash":              true,
		"safe-shell":        true,
		"tools":             true,
		"tools-safe-shell":  true,
		"tools-bash":        true,
		"tools-plus":        true,
	}
	if !validModes[c.Mode] {
		return fmt.Errorf("invalid mode: %s (must be one of: bash, safe-shell, tools, tools-safe-shell, tools-bash, tools-plus)", c.Mode)
	}

	return nil
}

// getEnvString gets a string from environment variable or returns default
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an integer from environment variable or returns default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

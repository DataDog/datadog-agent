package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the server configuration
type Config struct {
	Port           int
	KubeconfigPath string
	Context        string
	LogLevel       string
}

// Load loads configuration from command-line flags, environment variables, and defaults
func Load() (*Config, error) {
	cfg := &Config{}

	// Define command-line flags
	flag.IntVar(&cfg.Port, "port", getEnvInt("MCP_EVAL_PORT", 8080), "Port to listen on")
	flag.StringVar(&cfg.KubeconfigPath, "kubeconfig", getEnvString("MCP_EVAL_KUBECONFIG", defaultKubeconfigPath()), "Path to kubeconfig file")
	flag.StringVar(&cfg.Context, "context", getEnvString("MCP_EVAL_CONTEXT", "kind-mcp-eval"), "Kubernetes context to use")
	flag.StringVar(&cfg.LogLevel, "loglevel", getEnvString("MCP_EVAL_LOGLEVEL", "info"), "Log level (debug, info, warn, error)")

	flag.Parse()

	return cfg, cfg.validate()
}

// validate ensures the configuration is valid
func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be between 1 and 65535)", c.Port)
	}

	if c.KubeconfigPath == "" {
		return fmt.Errorf("kubeconfig path cannot be empty")
	}

	if c.Context == "" {
		return fmt.Errorf("kubernetes context cannot be empty")
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

	return nil
}

// defaultKubeconfigPath returns the default kubeconfig path
func defaultKubeconfigPath() string {
	// Check KUBECONFIG environment variable first
	if kubeconfigEnv := os.Getenv("KUBECONFIG"); kubeconfigEnv != "" {
		return kubeconfigEnv
	}

	// Fall back to ~/.kube/config
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
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

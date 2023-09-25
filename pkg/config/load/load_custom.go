package load

import (
	"errors"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/conf"
	"github.com/DataDog/datadog-agent/pkg/conf/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Load reads configs files and initializes the config module
func Load(cfg conf.Config, origin string, additionalKnownEnvVars []string) (*conf.Warnings, error) {
	return LoadDatadogCustom(cfg, origin, true, additionalKnownEnvVars)
}

// LoadWithoutSecret reads configs files, initializes the config module without decrypting any secrets
func LoadWithoutSecret(cfg conf.Config, origin string, additionalKnownEnvVars []string) (*conf.Warnings, error) {
	return LoadDatadogCustom(cfg, origin, false, additionalKnownEnvVars)
}

func LoadDatadogCustom(cfg conf.Config, origin string, loadSecret bool, additionalKnownEnvVars []string) (*conf.Warnings, error) {
	// Feature detection running in a defer func as it always  need to run (whether config load has been successful or not)
	// Because some Agents (e.g. trace-agent) will run even if config file does not exist
	defer func() {
		// Environment feature detection needs to run before applying override funcs
		// as it may provide such overrides
		env.DetectFeatures(cfg)
		conf.ApplyOverrideFuncs(cfg)
	}()

	warnings, err := LoadCustom(cfg, origin, loadSecret, additionalKnownEnvVars)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			log.Warnf("Error loading config: %v (check config file permissions for dd-agent user)", err)
		} else {
			log.Warnf("Error loading config: %v", err)
		}
		return warnings, err
	}

	err = checkConflictingOptions(cfg)
	if err != nil {
		return warnings, err
	}

	// If this variable is set to true, we'll use DefaultPython for the Python version,
	// ignoring the python_version configuration value.
	if ForceDefaultPython == "true" && cfg.IsKnown("python_version") {
		pv := cfg.GetString("python_version")
		if pv != DefaultPython {
			log.Warnf("Python version has been forced to %s", DefaultPython)
		}

		conf.AddOverride("python_version", DefaultPython)
	}

	conf.SanitizeAPIKeyConfig(cfg, "api_key")
	conf.SanitizeAPIKeyConfig(cfg, "logs_config.api_key")
	// setTracemallocEnabled *must* be called before setNumWorkers
	warnings.TraceMallocEnabledWithPy2 = setTracemallocEnabled(cfg)
	setNumWorkers(cfg)
	return warnings, setupFipsEndpoints(cfg)
}

// LoadCustom reads config into the provided config object
func LoadCustom(config conf.Config, origin string, loadSecret bool, additionalKnownEnvVars []string) (*conf.Warnings, error) {
	warnings := conf.Warnings{}

	if err := config.ReadInConfig(); err != nil {
		if env.IsServerless() {
			log.Debug("No config file detected, using environment variable based configuration only")
			return &warnings, nil
		}
		return &warnings, err
	}

	for _, key := range findUnknownKeys(config) {
		log.Warnf("Unknown key in config file: %v", key)
	}

	for _, v := range findUnknownEnvVars(config, os.Environ(), additionalKnownEnvVars) {
		log.Warnf("Unknown environment variable: %v", v)
	}

	for _, warningMsg := range findUnexpectedUnicode(config) {
		log.Warnf(warningMsg)
	}

	// We resolve proxy setting before secrets. This allows setting secrets through DD_PROXY_* env variables
	LoadProxyFromEnv(config)

	if loadSecret {
		if err := ResolveSecrets(config, origin); err != nil {
			return &warnings, err
		}
	}

	// Verify 'DD_URL' and 'DD_DD_URL' conflicts
	if EnvVarAreSetAndNotEqual("DD_DD_URL", "DD_URL") {
		log.Warnf("'DD_URL' and 'DD_DD_URL' variables are both set in environment. Using 'DD_DD_URL' value")
	}

	useHostEtc(config)
	return &warnings, nil
}

func findUnexpectedUnicode(config conf.Config) []string {
	messages := make([]string, 0)
	checkAndRecordString := func(str string, prefix string) {
		if res := FindUnexpectedUnicode(str); len(res) != 0 {
			for _, detected := range res {
				msg := fmt.Sprintf("%s - Unexpected unicode %s codepoint '%U' detected at byte position %v", prefix, detected.reason, detected.codepoint, detected.position)
				messages = append(messages, msg)
			}
		}
	}

	var visitElement func(string, interface{})
	visitElement = func(key string, element interface{}) {
		switch elementValue := element.(type) {
		case string:
			checkAndRecordString(elementValue, fmt.Sprintf("For key '%s', configuration value string '%s'", key, elementValue))
		case []string:
			for _, s := range elementValue {
				checkAndRecordString(s, fmt.Sprintf("For key '%s', configuration value string '%s'", key, s))
			}
		case []interface{}:
			for _, listItem := range elementValue {
				visitElement(key, listItem)
			}
		}
	}

	allKeys := config.AllKeys()
	for _, key := range allKeys {
		checkAndRecordString(key, fmt.Sprintf("Configuration key string '%s'", key))
		if unknownValue := config.Get(key); unknownValue != nil {
			visitElement(key, unknownValue)
		}
	}

	return messages
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:deadcode,unused
package appsec

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/appsec-internal-go/appsec"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	enabledEnvVar         = "DD_SERVERLESS_APPSEC_ENABLED"
	rulesEnvVar           = "DD_APPSEC_RULES"
	wafTimeoutEnvVar      = "DD_APPSEC_WAF_TIMEOUT"
	traceRateLimitEnvVar  = "DD_APPSEC_TRACE_RATE_LIMIT"
	obfuscatorKeyEnvVar   = "DD_APPSEC_OBFUSCATION_PARAMETER_KEY_REGEXP"
	obfuscatorValueEnvVar = "DD_APPSEC_OBFUSCATION_PARAMETER_VALUE_REGEXP"
)

const (
	defaultWAFTimeout           = 4 * time.Millisecond
	defaultTraceRate            = 100 // up to 100 appsec traces/s
	defaultObfuscatorKeyRegex   = `(?i)(?:p(?:ass)?w(?:or)?d|pass(?:_?phrase)?|secret|(?:api_?|private_?|public_?)key)|token|consumer_?(?:id|key|secret)|sign(?:ed|ature)|bearer|authorization`
	defaultObfuscatorValueRegex = `(?i)(?:p(?:ass)?w(?:or)?d|pass(?:_?phrase)?|secret|(?:api_?|private_?|public_?|access_?|secret_?)key(?:_?id)?|token|consumer_?(?:id|key|secret)|sign(?:ed|ature)?|auth(?:entication|orization)?)(?:\s*=[^;]|"\s*:\s*"[^"]+")|bearer\s+[a-z0-9\._\-]+|token:[a-z0-9]{13}|gh[opsu]_[0-9a-zA-Z]{36}|ey[I-L][\w=-]+\.ey[I-L][\w=-]+(?:\.[\w.+\/=-]+)?|[\-]{5}BEGIN[a-z\s]+PRIVATE\sKEY[\-]{5}[^\-]+[\-]{5}END[a-z\s]+PRIVATE\sKEY|ssh-rsa\s*[a-z0-9\/\.+]{100,}`
)

// StartOption is used to customize the AppSec configuration when invoked with appsec.Start()
type StartOption func(c *Config)

// Config is the AppSec configuration.
type Config struct {
	// rules loaded via the env var DD_APPSEC_RULES. When not set, the builtin rules will be used.
	rules []byte
	// Maximum WAF execution time
	wafTimeout time.Duration
	// AppSec trace rate limit (traces per second).
	traceRateLimit uint
	// Obfuscator configuration parameters
	obfuscator ObfuscatorConfig
}

// ObfuscatorConfig wraps the key and value regexp to be passed to the WAF to perform obfuscation.
type ObfuscatorConfig struct {
	KeyRegex   string
	ValueRegex string
}

// isEnabled returns true when appsec is enabled when the environment variable
// It also returns whether the env var is actually set in the env or not
// DD_APPSEC_ENABLED is set to true.
func isEnabled() (enabled bool, set bool, err error) {
	enabledStr, set := os.LookupEnv(enabledEnvVar)
	if enabledStr == "" {
		return false, set, nil
	} else if enabled, err = strconv.ParseBool(enabledStr); err != nil {
		return false, set, fmt.Errorf("could not parse %s value `%s` as a boolean value", enabledEnvVar, enabledStr)
	} else {
		return enabled, set, nil
	}
}

// IsStandalone returns whether appsec is used as a standalone product (without APM tracing) or not
func IsStandalone() bool {
	value, set := os.LookupEnv("DD_APM_TRACING_ENABLED")
	enabled, _ := strconv.ParseBool(value)
	return set && !enabled
}

func newConfig() (*Config, error) {
	rules, err := readRulesConfig()
	if err != nil {
		return nil, err
	}
	return &Config{
		rules:          rules,
		wafTimeout:     readWAFTimeoutConfig(),
		traceRateLimit: readRateLimitConfig(),
		obfuscator:     readObfuscatorConfig(),
	}, nil
}

func readWAFTimeoutConfig() (timeout time.Duration) {
	timeout = defaultWAFTimeout
	value := os.Getenv(wafTimeoutEnvVar)
	if value == "" {
		return
	}

	// Check if the value ends with a letter, which means the user has
	// specified their own time duration unit(s) such as 1s200ms.
	// Otherwise, default to microseconds.
	if lastRune, _ := utf8.DecodeLastRuneInString(value); !unicode.IsLetter(lastRune) {
		value += "us" // Add the default microsecond time-duration suffix
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		logEnvVarParsingError(wafTimeoutEnvVar, value, err, timeout)
		return
	}
	if parsed <= 0 {
		logUnexpectedEnvVarValue(wafTimeoutEnvVar, parsed, "expecting a strictly positive duration", timeout)
		return
	}
	return parsed
}

func readRateLimitConfig() (rate uint) {
	rate = defaultTraceRate
	value := os.Getenv(traceRateLimitEnvVar)
	if value == "" {
		return rate
	}
	parsed, err := strconv.ParseUint(value, 10, 0)
	if err != nil {
		logEnvVarParsingError(traceRateLimitEnvVar, value, err, rate)
		return
	}
	if rate == 0 {
		logUnexpectedEnvVarValue(traceRateLimitEnvVar, parsed, "expecting a value strictly greater than 0", rate)
		return
	}
	return uint(parsed)
}

func readObfuscatorConfig() ObfuscatorConfig {
	keyRE := readObfuscatorConfigRegexp(obfuscatorKeyEnvVar, defaultObfuscatorKeyRegex)
	valueRE := readObfuscatorConfigRegexp(obfuscatorValueEnvVar, defaultObfuscatorValueRegex)
	return ObfuscatorConfig{KeyRegex: keyRE, ValueRegex: valueRE}
}

func readObfuscatorConfigRegexp(name, defaultValue string) string {
	val, present := os.LookupEnv(name)
	if !present {
		log.Debugf("appsec: %s not defined, starting with the default obfuscator regular expression", name)
		return defaultValue
	}
	if _, err := regexp.Compile(val); err != nil {
		log.Errorf("appsec: could not compile the configured obfuscator regular expression `%s=%s`. Using the default value instead", name, val)
		return defaultValue
	}
	log.Debugf("appsec: starting with the configured obfuscator regular expression %s", name)
	return val
}

func readRulesConfig() (rules []byte, err error) {
	rules = []byte(appsec.StaticRecommendedRules)
	filepath := os.Getenv(rulesEnvVar)
	if filepath == "" {
		log.Info("appsec: starting with the default recommended security rules")
		return rules, nil
	}
	buf, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Errorf("appsec: could not find the rules file in path %s: %v.", filepath, err)
		}
		return nil, err
	}
	log.Info("appsec: starting with the security rules from file %s", filepath)
	return buf, nil
}

func logEnvVarParsingError(name, value string, err error, defaultValue interface{}) {
	log.Errorf("appsec: could not parse the env var %s=%s as a duration: %v. Using default value %v.", name, value, err, defaultValue)
}

func logUnexpectedEnvVarValue(name string, value interface{}, reason string, defaultValue interface{}) {
	log.Errorf("appsec: unexpected configuration value of %s=%v: %s. Using default value %v.", name, value, reason, defaultValue)
}

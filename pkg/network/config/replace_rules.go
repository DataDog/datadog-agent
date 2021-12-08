package config

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// ReplaceRule specifies a replace rule.
type ReplaceRule struct {
	// Pattern specifies the regexp pattern to be used when replacing. It must compile.
	Pattern string `mapstructure:"pattern"`

	// Re holds the compiled Pattern and is only used internally.
	Re *regexp.Regexp `mapstructure:"-"`

	// Repl specifies the replacement string to be used when Pattern matches.
	Repl string `mapstructure:"repl"`
}

func parseReplaceRules(cfg ddconfig.Config, key string) ([]*ReplaceRule, error) {
	if !config.Datadog.IsSet(key) {
		return nil, nil
	}

	rules := make([]*ReplaceRule, 0)
	if err := cfg.UnmarshalKey(key, &rules); err != nil {
		return nil, fmt.Errorf("rules format should be of the form '[{\"pattern\":\"pattern\",\"repl\":\"replace_str\"}]', error: %w", err)
	}

	for _, r := range rules {
		if r.Pattern == "" {
			return nil, errors.New(`all rules must have a "pattern"`)
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile %q: %s", r.Pattern, err)
		}
		r.Re = re
	}

	return rules, nil
}

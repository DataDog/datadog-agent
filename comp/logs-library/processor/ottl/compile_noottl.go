// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !otlp

// Package ottl compiles and evaluates OTTL rules for the logs pipeline.
//
// This file is used by builds that do not link the OTTL runtime (for example
// the IoT Agent). Regex processing rules keep working; any OTTL rule is
// rejected at validation time so it can never be silently skipped.
package ottl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// CompileProcessingRules validates rules and rejects OTTL rules in this build.
func CompileProcessingRules(rules []*config.ProcessingRule) error {
	if err := config.ValidateProcessingRules(rules); err != nil {
		return err
	}
	if err := config.CompileProcessingRules(rules); err != nil {
		return err
	}
	return rejectOTTLRules(rules)
}

func rejectOTTLRules(rules []*config.ProcessingRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case config.ExcludeAtMatchOTTL, config.IncludeAtMatchOTTL, config.MaskOTTLTransform:
			return fmt.Errorf("processing rule %q uses %q, which is not supported by this Agent build", rule.Name, rule.Type)
		}
	}
	return nil
}

// GlobalProcessingRules loads global rules and rejects OTTL rules in this build.
func GlobalProcessingRules(cfg pkgconfigmodel.Reader) ([]*config.ProcessingRule, error) {
	rules, err := config.GlobalProcessingRules(cfg)
	if err != nil {
		return nil, err
	}
	if err := rejectOTTLRules(rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// ValidateLogsConfig validates an integration's rules and rejects OTTL rules in this build.
func ValidateLogsConfig(cfg *config.LogsConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return rejectOTTLRules(cfg.ProcessingRules)
}

// PrepareSource rejects OTTL rules before LogSources publishes a source.
func PrepareSource(source *sources.LogSource) error {
	return rejectOTTLRules(source.Config.ProcessingRules)
}

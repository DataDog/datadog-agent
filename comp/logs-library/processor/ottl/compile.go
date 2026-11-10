// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ottl compiles and evaluates OTTL rules for the logs pipeline.
package ottl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type compiledRule struct {
	condition *OTTLCondition
	statement *OTTLStatement
}

func (*compiledRule) NewContext(content []byte) config.OTTLContext {
	return AcquireOTTLContext(content)
}

func (r *compiledRule) Evaluate(state config.OTTLContext) (bool, error) {
	c := state.(*OTTLContext)
	if r.statement != nil {
		return c.ApplyMask(context.Background(), r.statement)
	}
	return r.condition.Eval(context.Background(), c)
}

// Release returns the message-local context to its pool.
func (c *OTTLContext) Release() { ReleaseOTTLContext(c) }

// CompileProcessingRules fully validates and compiles rules before publication.
// Callers must not mutate rule definitions or compile them while processing logs.
func CompileProcessingRules(rules []*config.ProcessingRule) error {
	if err := config.ValidateProcessingRules(rules); err != nil {
		return err
	}
	if err := config.CompileProcessingRules(rules); err != nil {
		return err
	}
	return compileOTTLRules(rules)
}

func compileOTTLRules(rules []*config.ProcessingRule) error {
	for _, rule := range rules {
		if rule.OTTL != nil {
			continue
		}
		switch rule.Type {
		case config.ExcludeAtMatchOTTL, config.IncludeAtMatchOTTL:
			condition, err := compileOTTLCondition(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid OTTL condition for processing rule %q: %w", rule.Name, err)
			}
			rule.OTTL = &compiledRule{condition: condition}
		case config.MaskOTTLTransform:
			statement, err := compileOTTLMaskStatement(rule.Pattern)
			if err != nil {
				return fmt.Errorf("invalid OTTL masking statement for processing rule %q: %w", rule.Name, err)
			}
			rule.OTTL = &compiledRule{statement: statement}
		}
	}
	return nil
}

// GlobalProcessingRules loads and fully validates global rules at logs startup.
func GlobalProcessingRules(cfg pkgconfigmodel.Reader) ([]*config.ProcessingRule, error) {
	rules, err := config.GlobalProcessingRules(cfg)
	if err != nil {
		return nil, err
	}
	if err := compileOTTLRules(rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// ValidateLogsConfig validates and compiles an integration's rules before tailing.
func ValidateLogsConfig(cfg *config.LogsConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return compileOTTLRules(cfg.ProcessingRules)
}

// PrepareSource compiles OTTL before LogSources publishes a source to launchers.
// Structural and regex validation have already been performed by LogSources.
func PrepareSource(source *sources.LogSource) error {
	return compileOTTLRules(source.Config.ProcessingRules)
}

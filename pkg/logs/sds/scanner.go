// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sds

//nolint:revive
package sds

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	sds "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Currently the SDS scanner is not directly utilized by the agent. The last usage of this logic was in the logs-agent,
// which was removed in the 7.69.0 release. Associated PR: https://github.com/DataDog/datadog-agent/pull/36657
// This file is kept for future reference, in case we need to use the SDS scanner in another part of the agent.

const ScannedTag = "sds_agent:true"

const SDSEnabled = true

var (
	tlmSDSRulesState = telemetry.NewGaugeWithOpts("sds", "rules", []string{"pipeline", "state"},
		"Rules state.", telemetry.Options{DefaultMetric: true})
	tlmSDSReconfigError = telemetry.NewCounterWithOpts("sds", "reconfiguration_error", []string{"pipeline", "type", "error_type"},
		"Count of SDS reconfiguration error.", telemetry.Options{DefaultMetric: true})
	tlmSDSReconfigSuccess = telemetry.NewCounterWithOpts("sds", "reconfiguration_success", []string{"pipeline", "type"},
		"Count of SDS reconfiguration success.", telemetry.Options{DefaultMetric: true})
	tlmSDSProcessingLatency = telemetry.NewSimpleHistogram("sds", "processing_latency", "Processing latency histogram",
		[]float64{10, 250, 500, 2000, 5000, 10000}) // unit: us
)

// Scanner wraps an SDS Scanner implementation, adds reconfiguration
// capabilities and telemetry on top of it.
// Most of Scanner methods are not thread safe for performance reasons, the caller
// has to ensure of the thread safety.
type Scanner struct {
	*sds.Scanner
	// lock used to separate between the lifecycle of the scanner (Reconfigure, Delete)
	// and the use of the scanner (Scan).
	sync.Mutex
	// standard rules as received through the remote configuration, indexed
	// by the standard rule ID for O(1) access when receiving user configurations.
	standardRules map[string]StandardRuleConfig
	// standardDefaults contains some consts for using the standard rules definition.
	standardDefaults StandardRulesDefaults
	// rawConfig is the raw config previously received through RC.
	rawConfig []byte
	// configuredRules are stored on configuration to retrieve rules
	// information on match. Use this read-only.
	configuredRules []RuleConfig
	// pipelineID is the logs pipeline ID for which we've created this scanner,
	// stored as string as it is only used in the telemetry.
	pipelineID string
}

// CreateScanner creates an SDS scanner.
// Use `Reconfigure` to configure it manually.
func CreateScanner(pipelineID string) *Scanner {
	scanner := &Scanner{pipelineID: pipelineID}
	log.Debugf("creating a new SDS scanner (internal id: %p)", scanner)
	return scanner
}

// MatchActions as exposed by the RC configurations.
const (
	matchActionRCHash          = "hash"
	matchActionRCNone          = "none"
	matchActionRCPartialRedact = "partial_redact"
	matchActionRCRedact        = "redact"

	RCPartialRedactFirstCharacters = "first"
	RCPartialRedactLastCharacters  = "last"

	RCSecondaryValidationChineseIdChecksum = "chinese_id_checksum"
	RCSecondaryValidationLuhnChecksum      = "luhn_checksum"
)

// Reconfigure uses the given `ReconfigureOrder` to reconfigure in-memory
// standard rules or user configuration.
// The order contains both the kind of reconfiguration to do and the raw bytes
// to apply the reconfiguration.
// When receiving standard rules, user configuration are reloaded and scanners are
// recreated to use the newly received standard rules.
// The boolean return parameter indicates if the SDS scanner has been destroyed.
// This method is thread safe, a scan can't happen at the same time.
func (s *Scanner) Reconfigure(order ReconfigureOrder) (bool, error) {
	if s == nil {
		log.Warn("Trying to reconfigure a nil Scanner")
		return false, nil
	}

	s.Lock()
	defer s.Unlock()

	log.Debugf("Reconfiguring SDS scanner (internal id: %p)", s)

	switch order.Type {
	case StandardRules:
		// reconfigure the standard rules
		err := s.reconfigureStandardRules(order.Config)
		var isActive bool

		// if we already received a configuration and no errors happened while
		// reconfiguring the standard rules: reapply the user configuration now.
		if err == nil && s.rawConfig != nil {
			var rerr error
			if isActive, rerr = s.reconfigureRules(s.rawConfig); rerr != nil {
				log.Error("Can't reconfigure SDS after having received standard rules:", rerr)
				s.rawConfig = nil // we drop this configuration because it seems unusable
				if err == nil {
					err = rerr
				}
			}
		}
		return isActive, err
	case AgentConfig:
		return s.reconfigureRules(order.Config)
	case StopProcessing:
		return s.reconfigureRules([]byte("{}"))
	}

	return false, fmt.Errorf("Scanner.Reconfigure: Unknown order type: %v", order.Type)
}

// reconfigureStandardRules stores in-memory standard rules received through RC.
// This is NOT reconfiguring the internal SDS scanner, call `reconfigureRules`
// if you have to.
// This method is NOT thread safe, the caller has to ensure the thread safety.
func (s *Scanner) reconfigureStandardRules(rawConfig []byte) error {
	if rawConfig == nil {
		tlmSDSReconfigError.Inc(s.pipelineID, string(StandardRules), "nil_config")
		return fmt.Errorf("Invalid nil raw configuration for standard rules")
	}

	var unmarshaled StandardRulesConfig
	if err := json.Unmarshal(rawConfig, &unmarshaled); err != nil {
		tlmSDSReconfigError.Inc(s.pipelineID, string(StandardRules), "cant_unmarshal")
		return fmt.Errorf("Can't unmarshal raw configuration: %v", err)
	}

	// build a map for O(1) access when we'll receive configuration
	standardRules := make(map[string]StandardRuleConfig)
	for _, rule := range unmarshaled.Rules {
		standardRules[rule.ID] = rule
	}

	s.standardRules = standardRules
	s.standardDefaults = unmarshaled.Defaults

	tlmSDSReconfigSuccess.Inc(s.pipelineID, string(StandardRules))
	log.Info("Reconfigured", len(s.standardRules), "SDS standard rules.")
	for _, rule := range s.standardRules {
		log.Debug("Std rule:", rule.Name)
	}

	return nil
}

// reconfigureRules reconfigures the internal SDS scanner using the in-memory
// standard rules. Could possibly delete and recreate the internal SDS scanner if
// necessary.
// The boolean return parameter returns if an SDS scanner is active.
// This method is NOT thread safe, caller has to ensure the thread safety.
func (s *Scanner) reconfigureRules(rawConfig []byte) (bool, error) {
	if rawConfig == nil {
		tlmSDSReconfigError.Inc(s.pipelineID, string(AgentConfig), "nil_config")
		return s.Scanner != nil, fmt.Errorf("Invalid nil raw configuration received for user configuration")
	}

	if s.standardRules == nil || len(s.standardRules) == 0 {
		// store it for the next try
		s.rawConfig = rawConfig
		tlmSDSReconfigError.Inc(s.pipelineID, string(AgentConfig), "no_std_rules")
		log.Debug("Received an user configuration but no SDS standard rules available.")
		return s.Scanner != nil, nil
	}

	var config RulesConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		tlmSDSReconfigError.Inc(s.pipelineID, string(AgentConfig), "cant_unmarshal")
		return s.Scanner != nil, fmt.Errorf("Can't unmarshal raw configuration: %v", err)
	}

	// ignore disabled rules
	totalRulesReceived := len(config.Rules)
	config = config.OnlyEnabled()

	log.Infof("Starting an SDS reconfiguration: %d rules received (in which %d are disabled)", totalRulesReceived, totalRulesReceived-len(config.Rules))

	// if we received an empty array of rules or all rules disabled, interprets this as "stop SDS".
	if len(config.Rules) == 0 {
		log.Info("Received an empty configuration, stopping the SDS scanner.")
		// destroy the old scanner
		if s.Scanner != nil {
			s.Scanner.Delete()
			s.Scanner = nil
			s.rawConfig = rawConfig
			s.configuredRules = nil
			tlmSDSReconfigSuccess.Inc(s.pipelineID, "shutdown")
		}
		return false, nil
	}

	// prepare the scanner rules
	var sdsRules []sds.RuleConfig
	var malformedRulesCount int
	var unknownStdRulesCount int
	for _, userRule := range config.Rules {
		// read the rule in the standard rules
		standardRule, found := s.standardRules[userRule.Definition.StandardRuleID]
		if !found {
			log.Warnf("Referencing an unknown standard rule, id: %v", userRule.Definition.StandardRuleID)
			unknownStdRulesCount += 1
			continue
		}

		if rule, err := interpretRCRule(userRule, standardRule, s.standardDefaults); err != nil {
			// we warn that we can't interpret this rule, but we continue in order
			// to properly continue processing with the rest of the rules.
			malformedRulesCount += 1
			log.Warnf("%v", err.Error())
		} else {
			sdsRules = append(sdsRules, rule)
		}
	}

	tlmSDSRulesState.Set(float64(malformedRulesCount), s.pipelineID, "malformed")
	tlmSDSRulesState.Set(float64(unknownStdRulesCount), s.pipelineID, "unknown_std")

	// create the new SDS Scanner
	var scanner *sds.Scanner
	var err error
	if scanner, err = sds.CreateScanner(sdsRules); err != nil {
		tlmSDSReconfigError.Inc(s.pipelineID, string(AgentConfig), "scanner_error")
		return s.Scanner != nil, fmt.Errorf("while configuring an SDS Scanner: %v", err)
	}

	// destroy the old scanner
	if s.Scanner != nil {
		s.Scanner.Delete()
		s.Scanner = nil
	}

	// store the raw configuration for a later refresh
	// if we receive new standard rules
	s.rawConfig = rawConfig
	s.configuredRules = config.Rules

	log.Info("Created an SDS scanner with", len(scanner.RuleConfigs), "enabled rules")
	for _, rule := range s.configuredRules {
		log.Debug("Configured rule:", rule.Name)
	}
	s.Scanner = scanner

	tlmSDSRulesState.Set(float64(len(sdsRules)), s.pipelineID, "configured")
	tlmSDSRulesState.Set(float64(totalRulesReceived-len(config.Rules)), s.pipelineID, "disabled")
	tlmSDSReconfigSuccess.Inc(s.pipelineID, string(AgentConfig))

	return true, nil
}

// interpretRCRule interprets a rule as received through RC to return an sds.Rule usable with the shared library.
// `standardRule` contains the definition, with the name, pattern, etc.
// `userRule`     contains the configuration done by the user: match action, etc.
func interpretRCRule(userRule RuleConfig, standardRule StandardRuleConfig, defaults StandardRulesDefaults) (sds.RuleConfig, error) {
	var extraConfig sds.ExtraConfig

	var defToUse = StandardRuleDefinition{Version: -1}

	// go through all received definitions, use the most recent supported one.
	// O(n) number of definitions in the rule.
	for _, stdRuleDef := range standardRule.Definitions {
		if defToUse.Version > stdRuleDef.Version {
			continue
		}

		// The RC schema supports multiple of them,
		// for now though, the lib only supports one, so we'll just use the first one.
		reqCapabilitiesCount := len(stdRuleDef.RequiredCapabilities)
		if reqCapabilitiesCount > 0 {
			if reqCapabilitiesCount > 1 {
				log.Warnf("Standard rule '%v' with multiple required capabilities: %d. Only the first one will be used", standardRule.Name, reqCapabilitiesCount)
			}
			received := stdRuleDef.RequiredCapabilities[0]
			switch received {
			case RCSecondaryValidationChineseIdChecksum:
				extraConfig.SecondaryValidator = sds.ChineseIdChecksum
				defToUse = stdRuleDef
			case RCSecondaryValidationLuhnChecksum:
				extraConfig.SecondaryValidator = sds.LuhnChecksum
				defToUse = stdRuleDef
			default:
				// we don't know this required capability, test another version
				log.Warnf("unknown required capability: ", string(received))
				continue
			}
		} else {
			// no extra config to set
			defToUse = stdRuleDef
		}
	}

	if defToUse.Version == -1 {
		return nil, fmt.Errorf("unsupported rule with no compatible definition")
	}

	// If the "Use recommended keywords" checkbox has been checked, we use the default
	// included keywords available in the rule (curated by Datadog), if not included keywords
	// exist, fallback on using the default excluded keywords.
	// Otherwise:
	//   If some included keywords have been manually filled by the user, we use them
	//   Else we start using the default excluded keywords.
	if userRule.IncludedKeywords.UseRecommendedKeywords {
		// default included keywords if any
		if len(defToUse.DefaultIncludedKeywords) > 0 {
			extraConfig.ProximityKeywords = sds.CreateProximityKeywordsConfig(defaults.IncludedKeywordsCharCount, defToUse.DefaultIncludedKeywords, nil)
		} else if len(defaults.ExcludedKeywords) > 0 && defaults.ExcludedKeywordsCharCount > 0 {
			// otherwise fallback on default excluded keywords
			extraConfig.ProximityKeywords = sds.CreateProximityKeywordsConfig(defaults.ExcludedKeywordsCharCount, nil, defaults.ExcludedKeywords)
		}
	} else {
		if len(userRule.IncludedKeywords.Keywords) > 0 && userRule.IncludedKeywords.CharacterCount > 0 {
			// user provided included keywords
			extraConfig.ProximityKeywords = sds.CreateProximityKeywordsConfig(userRule.IncludedKeywords.CharacterCount, userRule.IncludedKeywords.Keywords, nil)
		} else if len(defaults.ExcludedKeywords) > 0 && defaults.ExcludedKeywordsCharCount > 0 {
			// default excluded keywords
			extraConfig.ProximityKeywords = sds.CreateProximityKeywordsConfig(defaults.ExcludedKeywordsCharCount, nil, defaults.ExcludedKeywords)
		} else {
			log.Warn("not using the recommended keywords but no keywords available for rule", userRule.Name)
		}
	}

	// we've compiled all necessary information merging the standard rule and the user config
	// create the rules for the scanner
	matchAction := strings.ToLower(userRule.MatchAction.Type)
	switch matchAction {
	case matchActionRCNone:
		return sds.NewMatchingRule(standardRule.Name, defToUse.Pattern, extraConfig), nil
	case matchActionRCRedact:
		return sds.NewRedactingRule(standardRule.Name, defToUse.Pattern, userRule.MatchAction.Placeholder, extraConfig), nil
	case matchActionRCPartialRedact:
		direction := sds.LastCharacters
		switch userRule.MatchAction.Direction {
		case string(RCPartialRedactLastCharacters):
			direction = sds.LastCharacters
		case string(RCPartialRedactFirstCharacters):
			direction = sds.FirstCharacters
		default:
			log.Warnf("Unknown PartialRedact direction (%v), falling back on LastCharacters", userRule.MatchAction.Direction)
		}
		return sds.NewPartialRedactRule(standardRule.Name, defToUse.Pattern, userRule.MatchAction.CharacterCount, direction, extraConfig), nil
	case matchActionRCHash:
		return sds.NewHashRule(standardRule.Name, defToUse.Pattern, extraConfig), nil
	}

	return nil, fmt.Errorf("Unknown MatchAction type (%v) received through RC for rule '%s':", matchAction, standardRule.Name)
}

// Scan scans the given `event` using the internal SDS scanner.
// Returns an error if the internal SDS scanner is not ready. If you need to
// validate that the internal SDS scanner can be used, use `IsReady()`.
// Returns a boolean indicating if the Scan has mutated the event and the returned
// one should be used instead.
// This method is thread safe, a reconfiguration can't happen at the same time.
func (s *Scanner) Scan(event []byte, msg *message.Message) (bool, []byte, error) {
	s.Lock()
	defer s.Unlock()
	start := time.Now()

	if s.Scanner == nil {
		return false, nil, fmt.Errorf("can't Scan with an unitialized scanner")
	}

	// scanning
	scanResult, err := s.Scanner.Scan(event)
	if len(scanResult.Matches) > 0 {
		for _, match := range scanResult.Matches {
			if rc, err := s.GetRuleByIdx(match.RuleIdx); err != nil {
				log.Warnf("can't apply rule tags: %v", err)
			} else {
				msg.ProcessingTags = append(msg.ProcessingTags, rc.Tags...)
			}
		}
	}
	// TODO(remy): in the future, we might want to do it differently than
	// using a tag.
	msg.ProcessingTags = append(msg.ProcessingTags, ScannedTag)

	tlmSDSProcessingLatency.Observe(float64(time.Since(start) / 1000))
	return scanResult.Mutated, scanResult.Event, err
}

// GetRuleByIdx returns the configured rule by its idx, referring to the idx
// that the SDS scanner writes in its internal response.
func (s *Scanner) GetRuleByIdx(idx uint32) (RuleConfig, error) {
	if s.Scanner == nil {
		return RuleConfig{}, fmt.Errorf("scanner not configured")
	}
	if uint32(len(s.configuredRules)) <= idx {
		return RuleConfig{}, fmt.Errorf("scanner not containing enough rules")
	}
	return s.configuredRules[idx], nil
}

// Delete deallocates the internal SDS scanner.
// This method is thread safe, a reconfiguration or a scan can't happen at the same time.
func (s *Scanner) Delete() {
	s.Lock()
	defer s.Unlock()

	if s.Scanner != nil {
		s.Scanner.Delete()
		s.rawConfig = nil
		s.configuredRules = nil
	}
	s.Scanner = nil
}

// IsReady returns true if this Scanner can be used
// to scan events and that at least one rule would be applied.
// This method is NOT thread safe, caller has to ensure the thread safety.
func (s *Scanner) IsReady() bool {
	if s == nil {
		return false
	}
	if s.Scanner == nil {
		return false
	}
	if len(s.Scanner.RuleConfigs) == 0 {
		return false
	}

	return true
}

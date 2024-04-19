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

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	sds "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"
)

const ScannedTag = "sds_agent:true"

const SDSEnabled = true

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
	// rawConfig is the raw config previously received through RC.
	rawConfig []byte
	// configuredRules are stored on configuration to retrieve rules
	// information on match. Use this read-only.
	configuredRules []RuleConfig
}

// CreateScanner creates an SDS scanner.
// Use `Reconfigure` to configure it manually.
func CreateScanner() *Scanner {
	scanner := &Scanner{}
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
// This method is thread safe, a scan can't happen at the same time.
func (s *Scanner) Reconfigure(order ReconfigureOrder) error {
	if s == nil {
		log.Warn("Trying to reconfigure a nil Scanner")
		return nil
	}

	s.Lock()
	defer s.Unlock()

	log.Debugf("Reconfiguring SDS scanner (internal id: %p)", s)

	switch order.Type {
	case StandardRules:
		err := s.reconfigureStandardRules(order.Config)
		// if we already received a configuration,
		// reapply it now that the standard rules have changed.
		if s.rawConfig != nil {
			if rerr := s.reconfigureRules(s.rawConfig); rerr != nil {
				log.Error("Can't reconfigure SDS after having received standard rules:", rerr)
				s.rawConfig = nil // we drop this configuration because it is unusable
				if err == nil {
					err = rerr
				}
			}
		}
		return err
	case AgentConfig:
		return s.reconfigureRules(order.Config)
	}

	return fmt.Errorf("Scanner.Reconfigure: Unknown order type: %v", order.Type)
}

// reconfigureStandardRules stores in-memory standard rules received through RC.
// This is NOT reconfiguring the internal SDS scanner, call `reconfigureRules`
// if you have to.
// This method is NOT thread safe, the caller has to ensure the thread safety.
func (s *Scanner) reconfigureStandardRules(rawConfig []byte) error {
	if rawConfig == nil {
		return fmt.Errorf("Invalid nil raw configuration for standard rules")
	}

	var unmarshaled StandardRulesConfig
	if err := json.Unmarshal(rawConfig, &unmarshaled); err != nil {
		return fmt.Errorf("Can't unmarshal raw configuration: %v", err)
	}

	// build a map for O(1) access when we'll receive configuration
	standardRules := make(map[string]StandardRuleConfig)
	for _, rule := range unmarshaled.Rules {
		standardRules[rule.ID] = rule
	}
	s.standardRules = standardRules

	log.Info("Reconfigured SDS standard rules.")
	return nil
}

// reconfigureRules reconfigures the internal SDS scanner using the in-memory
// standard rules. Could possibly delete and recreate the internal SDS scanner if
// necessary.
// This method is NOT thread safe, caller has to ensure the thread safety.
func (s *Scanner) reconfigureRules(rawConfig []byte) error {
	if rawConfig == nil {
		return fmt.Errorf("Invalid nil raw configuration received for user configuration")
	}

	if s.standardRules == nil || len(s.standardRules) == 0 {
		// store it for the next try
		s.rawConfig = rawConfig
		log.Info("Received an user configuration but no SDS standard rules available.")
		return nil
	}

	var config RulesConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return fmt.Errorf("Can't unmarshal raw configuration: %v", err)
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
			return nil
		}
		return nil
	}

	// prepare the scanner rules
	var sdsRules []sds.Rule
	for _, userRule := range config.Rules {
		// read the rule in the standard rules
		standardRule, found := s.standardRules[userRule.Definition.StandardRuleID]
		if !found {
			log.Warnf("Referencing an unknown standard rule, id: %v", userRule.Definition.StandardRuleID)
			continue
		}

		if rule, err := interpretRCRule(userRule, standardRule); err != nil {
			// we warn that we can't interpret this rule, but we continue in order
			// to properly continue processing with the rest of the rules.
			log.Warnf("%v", err.Error())
		} else {
			sdsRules = append(sdsRules, rule)
		}
	}

	// create the new SDS Scanner
	var scanner *sds.Scanner
	var err error
	if scanner, err = sds.CreateScanner(sdsRules); err != nil {
		return fmt.Errorf("while configuring an SDS Scanner: %v", err)
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

	log.Infof("Created an SDS scanner with %d enabled rules.", len(scanner.Rules))
	s.Scanner = scanner

	return nil
}

// interpretRCRule interprets a rule as received through RC to return
// an sds.Rule usable with the shared library.
// `standardRule` contains the definition, with the name, pattern, etc.
// `userRule`     contains the configuration done by the user: match action, etc.
func interpretRCRule(userRule RuleConfig, standardRule StandardRuleConfig) (sds.Rule, error) {
	var extraConfig sds.ExtraConfig

	// proximity keywords support
	if len(userRule.IncludedKeywords.Keywords) > 0 {
		extraConfig.ProximityKeywords = sds.CreateProximityKeywordsConfig(userRule.IncludedKeywords.CharacterCount, userRule.IncludedKeywords.Keywords, nil)
	}

	var name string = standardRule.Name
	var pattern string
	var usedVersion int = -1

	// go through all received definitions, use the most recent supported one.
	// O(n) number of definitions in the rule.
	// TODO(remy): this has to be refactored to avoid oversights in the switch
	for _, stdRuleDef := range standardRule.Definitions {
		if usedVersion > stdRuleDef.Version {
			continue
		}

		// The RC schema supports multiple of them,
		// for now though, the lib only supports one, so we'll just use the first one.
		if len(stdRuleDef.SecondaryValidators) > 0 {
			received := stdRuleDef.SecondaryValidators[0]
			switch received.Type {
			case RCSecondaryValidationChineseIdChecksum:
				extraConfig.SecondaryValidator = sds.ChineseIdChecksum
				pattern = stdRuleDef.Pattern
				usedVersion = stdRuleDef.Version
			case RCSecondaryValidationLuhnChecksum:
				extraConfig.SecondaryValidator = sds.LuhnChecksum
				pattern = stdRuleDef.Pattern
				usedVersion = stdRuleDef.Version
			default:
				// we don't know this secondary validator, test another version
				log.Warnf("unknown secondary validator: ", string(received.Type))
				continue
			}
		} else {
			pattern = stdRuleDef.Pattern
			usedVersion = stdRuleDef.Version
		}
	}

	if usedVersion == -1 {
		// TODO(remy): telemetry
		return sds.Rule{}, fmt.Errorf("unsupported rule, no compatible definition")
	}

	// we've compiled all necessary information merging the standard rule and the user config
	// create the rules for the scanner
	matchAction := strings.ToLower(userRule.MatchAction.Type)
	switch matchAction {
	case matchActionRCNone:
		return sds.NewMatchingRule(name, pattern, extraConfig), nil
	case matchActionRCRedact:
		return sds.NewRedactingRule(name, pattern, userRule.MatchAction.Placeholder, extraConfig), nil
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
		return sds.NewPartialRedactRule(name, pattern, userRule.MatchAction.CharacterCount, direction, extraConfig), nil
	case matchActionRCHash:
		return sds.NewHashRule(name, pattern, extraConfig), nil
	}

	return sds.Rule{}, fmt.Errorf("Unknown MatchAction type (%v) received through RC for rule '%s':", matchAction, name)
}

// Scan scans the given `event` using the internal SDS scanner.
// Returns an error if the internal SDS scanner is not ready. If you need to
// validate that the internal SDS scanner can be used, use `IsReady()`.
// This method is thread safe, a reconfiguration can't happen at the same time.
func (s *Scanner) Scan(event []byte, msg *message.Message) (bool, []byte, error) {
	s.Lock()
	defer s.Unlock()

	if s.Scanner == nil {
		return false, nil, fmt.Errorf("can't Scan with an unitialized scanner")
	}

	// scanning
	processed, rulesMatch, err := s.Scanner.Scan(event)
	matched := false
	if len(rulesMatch) > 0 {
		matched = true
		for _, match := range rulesMatch {
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

	return matched, processed, err
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
	if len(s.Scanner.Rules) == 0 {
		return false
	}

	return true
}

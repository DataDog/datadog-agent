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

	"github.com/DataDog/datadog-agent/pkg/util/log"
	sds "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"
)

// SDSEnabled is true when the Agent is compiled with the `sds` build tag and
// the shared library is available.
const SDSEnabled = true

// Scanner wraps an SDS Scanner implementation and adds reconfiguration
// capabilities on top of it.
// Most of Scanner methods are not thread safe for performance reasons, the caller
// has to ensure of the thread safety.
type Scanner struct {
	*sds.Scanner
	sync.Mutex

	// rawConfig is the raw user configuration previously received.
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

// Reconfigure uses the given `ReconfigureOrder` to reconfigure the SDS scanner.
// The order contains both the kind of reconfiguration to do and the raw bytes
// to apply the reconfiguration.
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
	case DatadogRules:
		return s.reconfigureRules(order.Config)
	}

	return fmt.Errorf("Scanner.Reconfigure: Unknown order type: %v", order.Type)
}

// reconfigureRules reconfigures the internal SDS scanner using the given set of
// self-contained rules (each rule carries its own pattern). Could possibly
// delete and recreate the internal SDS scanner if necessary.
// This method is NOT thread safe, caller has to ensure the thread safety.
func (s *Scanner) reconfigureRules(rawConfig []byte) error {
	if rawConfig == nil {
		return fmt.Errorf("Invalid nil raw configuration received for user configuration")
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
	var sdsRules []sds.RuleConfig
	for _, userRule := range config.Rules {
		// each rule is self-contained: it carries its own pattern and name.
		if userRule.Definition.Pattern == "" {
			log.Warnf("Rule '%s' (id: %v) has an empty pattern, skipping it", userRule.Name, userRule.ID)
			continue
		}

		var extraConfig sds.ExtraConfig
		if len(userRule.IncludedKeywords.Keywords) > 0 {
			extraConfig.ProximityKeywords = sds.CreateProximityKeywordsConfig(userRule.IncludedKeywords.CharacterCount, userRule.IncludedKeywords.Keywords, nil)
		}

		// create the rules for the scanner
		matchAction := strings.ToLower(userRule.MatchAction.Type)
		switch matchAction {
		case strings.ToLower(string(sds.MatchActionNone)):
			sdsRules = append(sdsRules, sds.NewMatchingRule(userRule.Name, userRule.Definition.Pattern, extraConfig))
		case strings.ToLower(string(sds.MatchActionRedact)):
			sdsRules = append(sdsRules, sds.NewRedactingRule(userRule.Name, userRule.Definition.Pattern, userRule.MatchAction.Placeholder, extraConfig))
		case strings.ToLower(string(sds.MatchActionPartialRedact)):
			direction := sds.LastCharacters
			switch userRule.MatchAction.Direction {
			case string(sds.LastCharacters):
				direction = sds.LastCharacters
			case string(sds.FirstCharacters):
				direction = sds.FirstCharacters
			default:
				log.Warnf("Unknown PartialRedact direction (%v), falling back on LastCharacters", userRule.MatchAction.Direction)
			}
			sdsRules = append(sdsRules, sds.NewPartialRedactRule(userRule.Name, userRule.Definition.Pattern, userRule.MatchAction.CharacterCount, direction, extraConfig))
		case strings.ToLower(string(sds.MatchActionHash)):
			sdsRules = append(sdsRules, sds.NewHashRule(userRule.Name, userRule.Definition.Pattern, extraConfig))
		default:
			log.Warnf("Unknown MatchAction type (%v) for rule '%s':", matchAction, userRule.Name)
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

	log.Infof("Created an SDS scanner with %d enabled rules.", len(scanner.RuleConfigs))
	s.Scanner = scanner

	return nil
}

// Scan scans the given `event` using the internal SDS scanner.
// It returns whether the event has been mutated (e.g. redacted), the processed
// event and an error if the internal SDS scanner is not ready. If you need to
// validate that the internal SDS scanner can be used, use `IsReady()`.
// When the event is not mutated, the returned processed slice is nil.
// This method is thread safe, a reconfiguration can't happen at the same time.
func (s *Scanner) Scan(event []byte) (bool, []byte, error) {
	s.Lock()
	defer s.Unlock()

	if s.Scanner == nil {
		return false, nil, fmt.Errorf("can't Scan with an uninitialized scanner")
	}

	result, err := s.Scanner.Scan(event)
	if err != nil {
		return false, nil, err
	}

	if !result.Mutated {
		return false, nil, nil
	}

	return true, result.Event, nil
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
// This method is NOT thread safe, caller has to ensure the thread safety.
func (s *Scanner) Delete() {
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scrubber

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"

	sdsLibrary "github.com/DataDog/dd-sensitive-data-scanner/sds-go/go"
)

// Rules definitions taken from https://github.com/DataDog/terraform-config/tree/master/internal-trust/sds-rules
var (
	agentDefaultReplacement = "********"
	sdsDefaultReplacement   = "[REDACTED]"
)

type SDSScrubber struct {
	sdsScanner *sdsLibrary.Scanner
	sdsRules   []sdsLibrary.RuleConfig
}

func initRules(additionalRules []sdsLibrary.RuleConfig) *SDSScrubber {
	preferedLookAheadCharacterCount := uint32(20)

	apiKeyExtraConfig := sdsLibrary.ExtraConfig{
		ProximityKeywords: sdsLibrary.CreateProximityKeywordsConfig(
			preferedLookAheadCharacterCount,
			[]string{
				"api_key=",
				"apikey=",
				"api_key:",
				"apikey:",
				"api_key:=",
				"apikey:=",
				"DD_API_KEY=",
			}, nil),
	}

	appKeyExtraConfig := sdsLibrary.ExtraConfig{
		ProximityKeywords: sdsLibrary.CreateProximityKeywordsConfig(
			preferedLookAheadCharacterCount,
			[]string{
				"app_key=",
				"appkey=",
				"application_key=",
				"app_key:",
				"appkey:",
				"application_key:",
				"app_key:=",
				"appkey:=",
				"application_key:=",
				"DD_APP_KEY=",
			}, nil),
	}

	ruleList := []sdsLibrary.RuleConfig{
		// Rule function available in the sds-go-library
		/*
		**  NewRedactingRule(id string, pattern string, redactionValue string, extraConfig ExtraConfig) RegexRuleConfig
		**  NewMatchingRule(id string, pattern string, extraConfig ExtraConfig) RegexRuleConfig
		**  NewHashRule(id string, pattern string, extraConfig ExtraConfig) RegexRuleConfig
		**  NewPartialRedactRule(id string, pattern string, characterCount uint32, direction PartialRedactionDirection, extraConfig ExtraConfig) RegexRuleConfig
		 */

		// API key rule
		sdsLibrary.NewPartialRedactRule("api_key", `\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`, 27, sdsLibrary.FirstCharacters, sdsLibrary.ExtraConfig{}),
		sdsLibrary.NewRedactingRule("api_key_hinted_min", `\b[a-fA-F0-9]{,31}\b`, agentDefaultReplacement, apiKeyExtraConfig),
		sdsLibrary.NewRedactingRule("api_key_hinted_max", `\b[a-fA-F0-9]{33,}\b`, agentDefaultReplacement, apiKeyExtraConfig),
		sdsLibrary.NewRedactingRule("api_key_hinted", `(api_?key=)\b[a-zA-Z0-9]+([a-zA-Z0-9]{5})\b`, "api_key="+agentDefaultReplacement, sdsLibrary.ExtraConfig{}),

		// APP key rule
		sdsLibrary.NewPartialRedactRule("appKeyReplacer", `\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`, 35, sdsLibrary.FirstCharacters, sdsLibrary.ExtraConfig{}),
		sdsLibrary.NewRedactingRule("app_key_hinted_min", `\b[a-fA-F0-9]{,39}\b`, agentDefaultReplacement, appKeyExtraConfig),
		sdsLibrary.NewRedactingRule("app_key_hinted_max", `\b[a-fA-F0-9]{41,}\b`, agentDefaultReplacement, appKeyExtraConfig),
		sdsLibrary.NewRedactingRule("app_key_hinted", `(ap(?:p|plication)_?key=)\b[a-zA-Z0-9]+([a-zA-Z0-9]{5})\b`, "app_key="+agentDefaultReplacement, sdsLibrary.ExtraConfig{}),
		sdsLibrary.NewRedactingRule("rcAppKeyReplacer", `\bDDRCM_[A-Z0-9]+([A-Z0-9]{5})\b`, agentDefaultReplacement, sdsLibrary.ExtraConfig{}),

		// Bearer token rule
		sdsLibrary.NewPartialRedactRule("hintedBearerReplacer", `[a-fA-F0-9]{59}([a-fA-F0-9]{5})`, 59, sdsLibrary.FirstCharacters, sdsLibrary.ExtraConfig{ProximityKeywords: sdsLibrary.CreateProximityKeywordsConfig(10, []string{"Bearer"}, nil)}),
		// extraction from SDS Standard Library
		sdsLibrary.NewRedactingRule("hintedBearerInvalidReplacer", `(?i)\bbearer [-a-z0-9._~+/]{4,}`, "Bearer"+" "+agentDefaultReplacement, sdsLibrary.ExtraConfig{}),

		// Password rule
		sdsLibrary.NewRedactingRule("uriPasswordReplacer", `(?i)([a-z][a-z0-9+-.]+://|\b)([^:]+):([^\s|"]+)@`, sdsDefaultReplacement+"@", sdsLibrary.ExtraConfig{}),
		sdsLibrary.NewRedactingRule("passwordReplacer", `(?i)(\"?(?:pass(?:word)?|pswd|pwd)\"?)((?:=| = |: )\"?)([0-9A-Za-z#!$%&()*+,\-./:<=>?@[\\\]^_{|}~]+)`, "password="+sdsDefaultReplacement, sdsLibrary.ExtraConfig{}),

		// Certificate rule
		sdsLibrary.NewRedactingRule("certReplacer", `-----BEGIN (?:.*)-----[A-Za-z0-9=\+\/\s]*-----END (?:.*)-----`, agentDefaultReplacement, sdsLibrary.ExtraConfig{}),
	}

	if additionalRules != nil {
		ruleList = append(ruleList, additionalRules...)
	}

	return &SDSScrubber{
		sdsRules: ruleList,
	}
}

func (sds *SDSScrubber) initObjectRules(additionalObjectRules []sdsLibrary.RuleConfig) *SDSScrubber {
	objectRuleList := []sdsLibrary.RuleConfig{
		sdsLibrary.NewRedactingRule("yamlPasswordReplacer", `(\s*(\w|_)*(pass(word)?|pwd)(\w|_)*\s*:)[^\n]*`, "password:"+sdsDefaultReplacement, sdsLibrary.ExtraConfig{}),
		sdsLibrary.NewRedactingRule("tokenReplacer", `(\s*(\w|_)*token\s*:).*`, "token:"+sdsDefaultReplacement, sdsLibrary.ExtraConfig{}),
		sdsLibrary.NewRedactingRule("snmpReplacer", `(\s*(community_string|authKey|privKey|community|authentication_key|privacy_key|Authorization|authorization)\s*:)[^\n]*`, "snmp_key:"+sdsDefaultReplacement, sdsLibrary.ExtraConfig{}),
		sdsLibrary.NewRedactingRule("snmpMultilineReplacer", `(\s*(community_strings)\s*:)\s*(?:\n(?:\s+-\s+.*)*|\[(?:\n?.*?)*?\])`, "community_strings: "+sdsDefaultReplacement, sdsLibrary.ExtraConfig{}),
	}

	sds.sdsRules = append(sds.sdsRules, objectRuleList...)
	if additionalObjectRules != nil {
		sds.sdsRules = append(sds.sdsRules, additionalObjectRules...)
	}
	return sds
}

func (sds *SDSScrubber) sdsStart() (*SDSScrubber, error) {
	var err error
	sds.sdsScanner, err = sdsLibrary.CreateScanner(sds.sdsRules)
	if err != nil {
		return nil, err
	}
	return sds, nil
}

func (sds *SDSScrubber) Scan(input []byte) ([]byte, error) {
	scanResult, err := sds.sdsScanner.Scan(input)
	if err != nil {
		return nil, err
	}
	return scanResult.Event, nil
}

func (sds *SDSScrubber) ScanObject(input map[string]interface{}) ([]byte, error) {
	scanResult, err := sds.sdsScanner.ScanEventsMap(input)
	if err != nil {
		return nil, err
	}
	return scanResult.Event, nil
}

func (sds *SDSScrubber) StartScan(filename string, input []byte) ([]byte, error) {
	var mapData map[string]interface{}
	var err error

	// Detect if the file is YAML
	if strings.HasPrefix(filename, "yaml") {
		err = yaml.Unmarshal(input, &mapData)
		if err != nil {
			return nil, err
		}
		// Print the map data
		return sds.ScanObject(mapData)
	}

	// Detect if the file is JSON
	if strings.HasPrefix(filename, "json") {
		err = json.Unmarshal(input, &mapData)
		if err != nil {
			return nil, err
		}
		return sds.ScanObject(mapData)
	}

	// For non-structured data (not JSON/YAML), do a raw scan
	return sds.Scan(input)
}

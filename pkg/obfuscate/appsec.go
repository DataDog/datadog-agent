// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"encoding/json"
	"regexp"
)

// ObfuscateAppSec obfuscates the given appsec tag value in order to remove sensitive values from the appsec security
// events. The tag value should be of the form `{"triggers":<appsec events>}` and follow the JSON schema defined at
// https://github.com/DataDog/libddwaf/blob/1.0.17/schema/appsec-event-1.0.0.json
func (o *Obfuscator) ObfuscateAppSec(val string) string {
	keyRE := o.opts.AppSec.ParameterKeyRegexp
	valueRE := o.opts.AppSec.ParameterValueRegexp
	if keyRE == nil && valueRE == nil {
		return val
	}

	var appsecMeta interface{}
	if err := json.Unmarshal([]byte(val), &appsecMeta); err != nil {
		o.log.Errorf("Could not parse the appsec span tag as a json value: %s", err)
		return val
	}

	meta, ok := appsecMeta.(map[string]interface{})
	if !ok {
		return val
	}

	triggers, ok := meta["triggers"].([]interface{})
	if !ok {
		return val
	}

	var sensitiveDataFound bool
	for _, trigger := range triggers {
		trigger, ok := trigger.(map[string]interface{})
		if !ok {
			continue
		}
		ruleMatches, ok := trigger["rule_matches"].([]interface{})
		if !ok {
			continue
		}
		for _, ruleMatch := range ruleMatches {
			ruleMatch, ok := ruleMatch.(map[string]interface{})
			if !ok {
				continue
			}
			parameters, ok := ruleMatch["parameters"].([]interface{})
			if !ok {
				continue
			}
			for _, param := range parameters {
				param, ok := param.(map[string]interface{})
				if !ok {
					continue
				}

				paramValue, hasStrValue := param["value"].(string)
				highlight, _ := param["highlight"].([]interface{})
				keyPath, _ := param["key_path"].([]interface{})

				var sensitiveKeyFound bool
				for _, key := range keyPath {
					str, ok := key.(string)
					if !ok {
						continue
					}
					if !matchString(keyRE, str) {
						continue
					}
					sensitiveKeyFound = true
					for i, v := range highlight {
						if _, ok := v.(string); ok {
							highlight[i] = "?"
						}
					}
					if hasStrValue {
						param["value"] = "?"
					}
					break
				}

				if sensitiveKeyFound {
					sensitiveDataFound = true
					continue
				}

				// Obfuscate the parameter value
				if hasStrValue && matchString(valueRE, paramValue) {
					sensitiveDataFound = true
					param["value"] = valueRE.ReplaceAllString(paramValue, "?")
				}

				// Obfuscate the parameter highlights
				for i, h := range highlight {
					h, ok := h.(string)
					if !ok {
						continue
					}
					if matchString(valueRE, h) {
						sensitiveDataFound = true
						highlight[i] = valueRE.ReplaceAllString(h, "?")
					}
				}
			}
		}
	}

	if !sensitiveDataFound {
		return val
	}

	newVal, err := json.Marshal(appsecMeta)
	if err != nil {
		o.log.Errorf("Could not marshal the obfuscated appsec span tag into a json value: %s", err)
		return val
	}
	return string(newVal)
}

func matchString(re *regexp.Regexp, s string) bool {
	if re == nil {
		return false
	}
	return re.MatchString(s)
}

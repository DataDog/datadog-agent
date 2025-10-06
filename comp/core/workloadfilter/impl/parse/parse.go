// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parse handles parsing and validating YAML configuration into CEL filter config
package parse

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// GetProductConfigs parses the configuration and returns raw rule strings organized by product
func GetProductConfigs(config workloadfilter.CELFilterConfig) (map[workloadfilter.Product]map[workloadfilter.ResourceType][]string, []error) {
	if config == nil {
		return nil, nil
	}

	productRulesMap := consolidateRulesByProduct(config)
	invalidConfigErrors := removeInvalidConfig(productRulesMap)

	return productRulesMap, invalidConfigErrors
}

// consolidateRulesByProduct merges all rules for the same product
func consolidateRulesByProduct(productRules []workloadfilter.RuleBundles) map[workloadfilter.Product]map[workloadfilter.ResourceType][]string {
	productRulesMap := make(map[workloadfilter.Product]map[workloadfilter.ResourceType][]string)

	for _, productRule := range productRules {
		for _, product := range productRule.Products {
			if productRulesMap[product] == nil {
				productRulesMap[product] = make(map[workloadfilter.ResourceType][]string)
			}
			for pluralResourceType, rules := range productRule.Rules {
				resourceType := pluralResourceType.ToSingular()
				productRulesMap[product][resourceType] = append(productRulesMap[product][resourceType], rules...)
			}
		}
	}

	return productRulesMap
}

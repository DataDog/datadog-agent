// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parse contains rule type definitions and product support mappings
package parse

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// ProductSupportMap defines which rule types each product supports
// Maps from our input Product types to workloadfilter ResourceTypes
var ProductSupportMap = map[workloadfilter.Product]map[workloadfilter.ResourceType]struct{}{
	workloadfilter.ProductMetrics: {
		workloadfilter.ContainerType: {},
		workloadfilter.PodType:       {},
		workloadfilter.ServiceType:   {},
		workloadfilter.EndpointType:  {},
	},
	workloadfilter.ProductLogs: {
		workloadfilter.ContainerType: {},
		workloadfilter.PodType:       {},
	},
	workloadfilter.ProductSBOM: {
		workloadfilter.ContainerType: {},
	},
	workloadfilter.ProductGlobal: {
		workloadfilter.ContainerType: {},
		workloadfilter.PodType:       {},
		workloadfilter.ServiceType:   {},
		workloadfilter.EndpointType:  {},
	},
}

func isRuleTypeSupported(product workloadfilter.Product, resourceType workloadfilter.ResourceType) bool {
	productSupport, exists := ProductSupportMap[product]
	if !exists {
		return false
	}
	_, supported := productSupport[resourceType]
	return supported
}

func logValidationWarnings(product workloadfilter.Product, rules map[workloadfilter.ResourceType][]string) {
	// Check each rule type using loop
	for resourceType, ruleStrings := range rules {
		if len(ruleStrings) > 0 && !isRuleTypeSupported(product, resourceType) {
			log.Warnf("Product %s does not support %s rules (found %d rules)\n",
				product, resourceType, len(ruleStrings))
		}
	}
}

func checkUndefinedProducts(productRules map[workloadfilter.Product]map[workloadfilter.ResourceType][]string) []string {
	undefinedProducts := []string{}
	productSet := makeSet(workloadfilter.GetAllProducts())

	for product := range productRules {
		if _, exists := productSet[product]; !exists {
			undefinedProducts = append(undefinedProducts, string(product))
		}
	}

	return undefinedProducts
}

func checkUndefinedResourceTypes(productRules map[workloadfilter.Product]map[workloadfilter.ResourceType][]string) []string {
	undefinedResourceTypes := []string{}
	resourceTypeSet := makeSet(workloadfilter.GetAllResourceTypes())

	for _, rules := range productRules {
		for resourceType := range rules {
			if _, exists := resourceTypeSet[resourceType]; !exists {
				undefinedResourceTypes = append(undefinedResourceTypes, string(resourceType))
			}
		}
	}

	return undefinedResourceTypes
}

func makeSet[T comparable](items []T) map[T]struct{} {
	set := make(map[T]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package parse contains rule type definitions and product support mappings
package parse

import (
	"errors"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// ProductSupportMap defines which rule types each product supports
var ProductSupportMap = map[workloadfilter.Product]map[workloadfilter.ResourceType]struct{}{
	workloadfilter.ProductMetrics: {
		workloadfilter.ContainerType:    {},
		workloadfilter.PodType:          {},
		workloadfilter.KubeServiceType:  {},
		workloadfilter.KubeEndpointType: {},
	},
	workloadfilter.ProductLogs: {
		workloadfilter.ContainerType: {},
		workloadfilter.ProcessType:   {},
	},
	workloadfilter.ProductSBOM: {
		workloadfilter.ContainerType: {},
	},
	workloadfilter.ProductGlobal: {
		workloadfilter.ContainerType:    {},
		workloadfilter.PodType:          {},
		workloadfilter.KubeServiceType:  {},
		workloadfilter.KubeEndpointType: {},
		workloadfilter.ProcessType:      {},
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

func removeInvalidConfig(productRules map[workloadfilter.Product]map[workloadfilter.ResourceType][]string) []error {
	var unsupportedErrors []error
	resourceTypeSet := makeSet(workloadfilter.GetAllResourceTypes())
	productSet := makeSet(workloadfilter.GetAllProducts())

	for product, rules := range productRules {
		// Remove unsupported product
		if _, exists := productSet[product]; !exists {
			unsupportedErrors = append(unsupportedErrors, errors.New("unsupported product for CEL workload filtering: "+string(product)))
			delete(productRules, product)
			continue
		}
		for resourceType, ruleStrings := range rules {
			// Remove unsupported resource type
			if _, exists := resourceTypeSet[resourceType]; !exists {
				unsupportedErrors = append(unsupportedErrors, errors.New("unsupported resource type for CEL workload filtering: "+string(resourceType)))
				delete(rules, resourceType)
			}
			// Remove product specific unsupported resource type
			if len(ruleStrings) > 0 && !isRuleTypeSupported(product, resourceType) {
				unsupportedErrors = append(unsupportedErrors, errors.New("unsupported resource type for CEL workload filtering on "+string(product)+" product: "+string(resourceType)))
				delete(rules, resourceType)
			}
		}
	}

	return unsupportedErrors
}

func makeSet[T comparable](items []T) map[T]struct{} {
	set := make(map[T]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

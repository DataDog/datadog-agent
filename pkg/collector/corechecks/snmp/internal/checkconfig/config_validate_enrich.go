// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

var validMetadataResources = map[string]map[string]bool{
	"device": {
		"name":          true,
		"description":   true,
		"sys_object_id": true,
		"location":      true,
		"serial_number": true,
		"vendor":        true,
		"version":       true,
		"product_name":  true,
		"model":         true,
		"os_name":       true,
		"os_version":    true,
		"os_hostname":   true,
	},
	"interface": {
		"name":         true,
		"alias":        true,
		"description":  true,
		"mac_address":  true,
		"admin_status": true,
		"oper_status":  true,
	},
}

type SymbolContext int64

const (
	ScalarSymbol SymbolContext = iota
	ColumnSymbol
	MetricTagSymbol
	MetadataSymbol
)

// ValidateEnrichMetricTags validates and enrich metric tags
func ValidateEnrichMetricTags(metricTags []profiledefinition.MetricTagConfig) []string {
	var errors []string
	for i := range metricTags {
		errors = append(errors, validateEnrichMetricTag(&metricTags[i])...)
	}
	return errors
}

// ValidateEnrichMetrics will validate MetricsConfig and enrich it.
// Example of enrichment:
// - storage of compiled regex pattern
func ValidateEnrichMetrics(metrics []profiledefinition.MetricsConfig) []string {
	var errors []string
	for i := range metrics {
		metricConfig := &metrics[i]
		if !metricConfig.IsScalar() && !metricConfig.IsColumn() {
			errors = append(errors, fmt.Sprintf("either a table symbol or a scalar symbol must be provided: %#v", metricConfig))
		}
		if metricConfig.IsScalar() && metricConfig.IsColumn() {
			errors = append(errors, fmt.Sprintf("table symbol and scalar symbol cannot be both provided: %#v", metricConfig))
		}
		if metricConfig.IsScalar() {
			errors = append(errors, validateEnrichSymbol(&metricConfig.Symbol, ScalarSymbol)...)
		}
		if metricConfig.IsColumn() {
			for j := range metricConfig.Symbols {
				errors = append(errors, validateEnrichSymbol(&metricConfig.Symbols[j], ColumnSymbol)...)
			}
			if len(metricConfig.MetricTags) == 0 {
				errors = append(errors, fmt.Sprintf("column symbols doesn't have a 'metric_tags' section (%+v), all its metrics will use the same tags; "+
					"if the table has multiple rows, only one row will be submitted; "+
					"please add at least one discriminating metric tag (such as a row index) "+
					"to ensure metrics of all rows are submitted", metricConfig.Symbols))
			}
			for i := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[i]
				errors = append(errors, validateEnrichMetricTag(metricTag)...)
			}
		}
		// Setting forced_type value to metric_type value for backward compatibility
		if metricConfig.MetricType == "" && metricConfig.ForcedType != "" {
			metricConfig.MetricType = metricConfig.ForcedType
		}
		metricConfig.ForcedType = ""
	}
	return errors
}

// validateEnrichMetadata will validate MetadataConfig and enrich it.
func validateEnrichMetadata(metadata profiledefinition.MetadataConfig) []string {
	var errors []string
	for resName := range metadata {
		_, isValidRes := validMetadataResources[resName]
		if !isValidRes {
			errors = append(errors, fmt.Sprintf("invalid resource: %s", resName))
		} else {
			res := metadata[resName]
			for fieldName := range res.Fields {
				_, isValidField := validMetadataResources[resName][fieldName]
				if !isValidField {
					errors = append(errors, fmt.Sprintf("invalid resource (%s) field: %s", resName, fieldName))
					continue
				}
				field := res.Fields[fieldName]
				for i := range field.Symbols {
					errors = append(errors, validateEnrichSymbol(&field.Symbols[i], MetadataSymbol)...)
				}
				if field.Symbol.OID != "" {
					errors = append(errors, validateEnrichSymbol(&field.Symbol, MetadataSymbol)...)
				}
				res.Fields[fieldName] = field
			}
			metadata[resName] = res
		}
		if resName == "device" && len(metadata[resName].IDTags) > 0 {
			errors = append(errors, "device resource does not support custom id_tags")
		}
		for i := range metadata[resName].IDTags {
			metricTag := &metadata[resName].IDTags[i]
			errors = append(errors, validateEnrichMetricTag(metricTag)...)
		}
	}
	return errors
}

func validateEnrichSymbol(symbol *profiledefinition.SymbolConfig, symbolContext SymbolContext) []string {
	var errors []string
	if symbol.Name == "" {
		errors = append(errors, fmt.Sprintf("symbol name missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
	}
	if symbol.OID == "" {
		if symbolContext == ColumnSymbol && !symbol.ConstantValueOne {
			errors = append(errors, fmt.Sprintf("symbol oid or send_as_one missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
		} else if symbolContext != ColumnSymbol {
			errors = append(errors, fmt.Sprintf("symbol oid missing: name=`%s` oid=`%s`", symbol.Name, symbol.OID))
		}
	}
	if symbol.ExtractValue != "" {
		pattern, err := regexp.Compile(symbol.ExtractValue)
		if err != nil {
			errors = append(errors, fmt.Sprintf("cannot compile `extract_value` (%s): %s", symbol.ExtractValue, err.Error()))
		} else {
			symbol.ExtractValueCompiled = pattern
		}
	}
	if symbol.MatchPattern != "" {
		pattern, err := regexp.Compile(symbol.MatchPattern)
		if err != nil {
			errors = append(errors, fmt.Sprintf("cannot compile `extract_value` (%s): %s", symbol.ExtractValue, err.Error()))
		} else {
			symbol.MatchPatternCompiled = pattern
		}
	}
	if symbolContext != ColumnSymbol && symbol.ConstantValueOne {
		errors = append(errors, "`constant_value_one` cannot be used outside of tables")
	}
	if (symbolContext != ColumnSymbol && symbolContext != ScalarSymbol) && symbol.MetricType != "" {
		errors = append(errors, "`metric_type` cannot be used outside scalar/table metric symbols and metrics root")
	}
	return errors
}
func validateEnrichMetricTag(metricTag *profiledefinition.MetricTagConfig) []string {
	var errors []string
	if metricTag.Column.OID != "" || metricTag.Column.Name != "" {
		errors = append(errors, validateEnrichSymbol(&metricTag.Column, MetricTagSymbol)...)
	}
	if metricTag.Match != "" {
		pattern, err := regexp.Compile(metricTag.Match)
		if err != nil {
			errors = append(errors, fmt.Sprintf("cannot compile `match` (`%s`): %s", metricTag.Match, err.Error()))
		} else {
			metricTag.Pattern = pattern
		}
		if len(metricTag.Tags) == 0 {
			errors = append(errors, fmt.Sprintf("`tags` mapping must be provided if `match` (`%s`) is defined", metricTag.Match))
		}
	}
	if len(metricTag.Mapping) > 0 && metricTag.Tag == "" {
		log.Warnf("``tag` must be provided if `mapping` (`%s`) is defined", metricTag.Mapping)
	}
	for _, transform := range metricTag.IndexTransform {
		if transform.Start > transform.End {
			errors = append(errors, fmt.Sprintf("transform rule end should be greater than start. Invalid rule: %#v", transform))
		}
	}
	return errors
}

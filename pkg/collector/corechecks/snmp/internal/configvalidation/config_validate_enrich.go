// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configvalidation contains validation and enrichment functions
package configvalidation

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
)

// SymbolContext represent the context in which the symbol is used
type SymbolContext int64

// ScalarSymbol enums
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

// ValidateEnrichMetadata will validate MetadataConfig and enrich it.
// no need to validate the metadata fields as the profile definition already does that
func ValidateEnrichMetadata(metadata profiledefinition.MetadataConfig) []string {
	var errors []string
	// iterate over the fields of metadata to get Device & Interface
	v := reflect.ValueOf(metadata)
	for i := 0; i < v.NumField(); i++ {
		resourceName := v.Type().Field(i).Name
		if resourceName == "Device" {
			resource := v.Field(i).Interface().(profiledefinition.DeviceMetadata)
			// iterate over the fields of the DeviceMetadata struct
			for j := 0; j < reflect.ValueOf(resource).NumField(); j++ {
				field := reflect.ValueOf(resource).Field(j).Interface().(profiledefinition.MetadataField)
				err, field := validateMetadataField(&field)
				if err != nil {
					errors = append(errors, err...)
				}
				reflect.ValueOf(&resource).Elem().Field(j).Set(reflect.ValueOf(field))
			}
			reflect.ValueOf(&metadata).Elem().Field(i).Set(reflect.ValueOf(resource))
		} else if resourceName == "Interface" {
			resource := v.Field(i).Interface().(profiledefinition.InterfaceMetadata)
			// iterate over the fields of the InterfaceMetadata struct
			for j := 0; j < reflect.ValueOf(resource).NumField(); j++ {
				field := reflect.ValueOf(resource).Field(j).Interface().(profiledefinition.MetadataField)
				err, field := validateMetadataField(&field)
				if err != nil {
					errors = append(errors, err...)
				}
				reflect.ValueOf(&resource).Elem().Field(j).Set(reflect.ValueOf(field))
			}
			for i := range resource.IDTags {
				metricTag := &resource.IDTags[i]
				errors = append(errors, validateEnrichMetricTag(metricTag)...)
				resource.IDTags[i] = *metricTag
			}
			reflect.ValueOf(&metadata).Elem().Field(i).Set(reflect.ValueOf(resource))
		} else {
			// we only expect two types of resources: Device & Interface
			// if this is not the case, log an error and return the metadataStore as is
			return []string{fmt.Sprintf("unexpected resource type: %s", resourceName)}
		}
	}
	return errors
}

func validateMetadataField(field *profiledefinition.MetadataField) ([]string, profiledefinition.MetadataField) {
	var errors []string
	if field.Symbol.OID != "" {
		errors = append(errors, validateEnrichSymbol(&field.Symbol, MetadataSymbol)...)
	}
	for i := range field.Symbols {
		errors = append(errors, validateEnrichSymbol(&field.Symbols[i], MetadataSymbol)...)
	}
	return errors, *field
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
	if (metricTag.Column.OID != "" || metricTag.Column.Name != "") && (metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "") {
		errors = append(errors, fmt.Sprintf("metric tag symbol and column cannot be both declared: symbol=%v, column=%v", metricTag.Symbol, metricTag.Column))
	}

	// Move deprecated metricTag.Column to metricTag.Symbol
	if metricTag.Column.OID != "" || metricTag.Column.Name != "" {
		metricTag.Symbol = profiledefinition.SymbolConfigCompat(metricTag.Column)
		metricTag.Column = profiledefinition.SymbolConfig{}
	}

	// OID/Name to Symbol harmonization:
	// When users declare metric tag like:
	//   metric_tags:
	//     - OID: 1.2.3
	//       symbol: aSymbol
	// this will lead to OID stored as MetricTagConfig.OID  and name stored as MetricTagConfig.Symbol.Name
	// When this happens, we harmonize by moving MetricTagConfig.OID to MetricTagConfig.Symbol.OID.
	if metricTag.OID != "" && metricTag.Symbol.OID != "" {
		errors = append(errors, fmt.Sprintf("metric tag OID and symbol.OID cannot be both declared: OID=%s, symbol.OID=%s", metricTag.OID, metricTag.Symbol.OID))
	}
	if metricTag.OID != "" && metricTag.Symbol.OID == "" {
		metricTag.Symbol.OID = metricTag.OID
		metricTag.OID = ""
	}
	if metricTag.Symbol.OID != "" || metricTag.Symbol.Name != "" {
		symbol := profiledefinition.SymbolConfig(metricTag.Symbol)
		errors = append(errors, validateEnrichSymbol(&symbol, MetricTagSymbol)...)
		metricTag.Symbol = profiledefinition.SymbolConfigCompat(symbol)
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
		errors = append(errors, fmt.Sprintf("``tag` must be provided if `mapping` (`%s`) is defined", metricTag.Mapping))
	}
	for _, transform := range metricTag.IndexTransform {
		if transform.Start > transform.End {
			errors = append(errors, fmt.Sprintf("transform rule end should be greater than start. Invalid rule: %#v", transform))
		}
	}
	return errors
}

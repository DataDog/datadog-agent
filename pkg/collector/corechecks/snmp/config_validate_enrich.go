package snmp

import (
	"fmt"
	"regexp"
)

func validateEnrichMetricTags(metricTags []metricTagConfig) []string {
	var errors []string
	for i := range metricTags {
		errors = append(errors, validateEnrichMetricTag(&metricTags[i], metricsConfig{})...)
	}
	return errors
}

func validateEnrichMetrics(metrics []metricsConfig) []string {
	var errors []string
	for i := range metrics {
		metricConfig := metrics[i]
		if metricConfig.Symbol.OID == "" && len(metricConfig.Symbols) == 0 {
			errors = append(errors, fmt.Sprintf("either a table symbol or a scalar symbol must be provided: %#v", metricConfig))
		}
		if metricConfig.Symbol.OID != "" && len(metricConfig.Symbols) > 0 {
			errors = append(errors, fmt.Sprintf("table symbol and scalar symbol cannot be both provided: %#v", metricConfig))
		}
		if metricConfig.Symbol.OID != "" {
			errors = append(errors, validateSymbol(metricConfig.Symbol, metricConfig)...)
		}
		if len(metricConfig.Symbols) > 0 {
			for _, symbol := range metricConfig.Symbols {
				errors = append(errors, validateSymbol(symbol, metricConfig)...)
			}
			if len(metricConfig.MetricTags) == 0 {
				errors = append(errors, fmt.Sprintf("column symbols %v doesn't have a 'metric_tags' section, all its metrics will use the same tags; "+
					"if the table has multiple rows, only one row will be submitted; "+
					"please add at least one discriminating metric tag (such as a row index) "+
					"to ensure metrics of all rows are submitted", metricConfig.Symbols))
			}
			for i := range metricConfig.MetricTags {
				metricTag := &metricConfig.MetricTags[i]
				errors = append(errors, validateEnrichMetricTag(metricTag, metricConfig)...)
			}
		}
	}
	return errors
}

func validateSymbol(symbol symbolConfig, metricConfig metricsConfig) []string {
	var errors []string
	if symbol.Name == "" {
		errors = append(errors, fmt.Sprintf("symbol name missing: name=`%s` oid=`%s`: %#v", symbol.Name, symbol.OID, metricConfig))
	}
	if symbol.OID == "" {
		errors = append(errors, fmt.Sprintf("symbol oid missing: name=`%s` oid=`%s`: %#v", symbol.Name, symbol.OID, metricConfig))
	}
	return errors
}
func validateEnrichMetricTag(metricTag *metricTagConfig, metricConfig metricsConfig) []string {
	var errors []string
	if metricTag.Column.OID != "" || metricTag.Column.Name != "" {
		errors = append(errors, validateSymbol(metricTag.Column, metricConfig)...)
	}
	if metricTag.Match != "" {
		pattern, err := regexp.Compile(metricTag.Match)
		if err != nil {
			errors = append(errors, fmt.Sprintf("cannot compile `match` (`%s`): %s : %#v", metricTag.Match, err.Error(), metricConfig))
		} else {
			metricTag.pattern = pattern
		}
		if len(metricTag.Tags) == 0 {
			errors = append(errors, fmt.Sprintf("`tags` mapping must be provided if `match` (`%s`) is defined: %#v", metricTag.Match, metricConfig))
		}
	}
	for _, transform := range metricTag.IndexTransform {
		if transform.Start > transform.End {
			errors = append(errors, fmt.Sprintf("transform rule end should be greater than start. Invalid rule: %#v", transform))
		}
	}
	return errors
}

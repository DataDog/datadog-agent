package snmp

import (
	"fmt"
)

func validateMetrics(metrics []metricsConfig) []error {
	var errors []error
	for _, metricConfig := range metrics {
		if metricConfig.MIB == "" {
			errors = append(errors, fmt.Errorf("MIB must be provided: %#v", metricConfig))
		}
		if metricConfig.Symbol.OID == "" && metricConfig.Table.OID == "" {
			errors = append(errors, fmt.Errorf("either a table symbol or a scalar symbol must be provided: %#v", metricConfig))
		}
		if metricConfig.Symbol.OID != "" && metricConfig.Table.OID != "" {
			errors = append(errors, fmt.Errorf("table symbol and scalar symbol cannot be both provided: %#v", metricConfig))
		}
		if metricConfig.Symbol.OID != "" {
			errors = append(errors, validateSymbol(metricConfig.Symbol, metricConfig)...)
		}
		if metricConfig.Table.OID != "" {
			if len(metricConfig.Symbols) == 0 {
				errors = append(errors, fmt.Errorf("when using table, a list of column symbols must be provided: %#v", metricConfig))
			}
			for _, symbol := range metricConfig.Symbols {
				errors = append(errors, validateSymbol(symbol, metricConfig)...)
			}
			if len(metricConfig.MetricTags) == 0 {
				errors = append(errors, fmt.Errorf("table %s doesn't have a 'metric_tags' section, all its metrics will use the same tags; "+
					"if the table has multiple rows, only one row will be submitted; "+
					"please add at least one discriminating metric tag (such as a row index) "+
					"to ensure metrics of all rows are submitted", metricConfig.Table.Name))
			}
			for _, metricTag := range metricConfig.MetricTags {
				errors = append(errors, validateMetricTag(metricTag, metricConfig)...)
			}
		}
	}
	return errors
}

func validateSymbol(symbol symbolConfig, metricConfig metricsConfig) []error {
	var errors []error
	if symbol.Name == "" {
		errors = append(errors, fmt.Errorf("symbol name missing: name=`%s` oid=`%s`: %#v", symbol.Name, symbol.OID, metricConfig))
	}
	if symbol.OID == "" {
		errors = append(errors, fmt.Errorf("symbol oid missing: name=`%s` oid=`%s`: %#v", symbol.Name, symbol.OID, metricConfig))
	}
	return errors
}
func validateMetricTag(metricTag metricTagConfig, metricConfig metricsConfig) []error {
	var errors []error
	if metricTag.Column.OID != "" || metricTag.Column.Name != "" {
		errors = append(errors, validateSymbol(metricTag.Column, metricConfig)...)
	}
	return errors
}

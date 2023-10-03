// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

// EvaluatedSampleDependencies set of supported memory usage metrics
var EvaluatedSampleDependencies = map[string]bool{
	"memory.usage": true,
	"memory.used":  true,
	"memory.total": true,
	"memory.free":  true,
}

func (ms *MetricSender) tryReportMemoryUsage(scalarSamples map[string]MetricSample, columnSamples map[string]map[string]MetricSample) error {
	if ms.hasScalarMemoryUsage(scalarSamples) {
		return ms.trySendScalarMemoryUsage(scalarSamples)
	}

	return ms.trySendColumnMemoryUsage(columnSamples)
}

// hasScalarMemoryUsage is true if samples store contains at least one memory usage related metric
func (ms *MetricSender) hasScalarMemoryUsage(scalarSamples map[string]MetricSample) bool {
	_, scalarMemoryUsageOk := scalarSamples["memory.usage"]
	_, scalarMemoryUsedOk := scalarSamples["memory.used"]
	_, scalarMemoryTotalOk := scalarSamples["memory.total"]
	_, scalarMemoryFreeOk := scalarSamples["memory.free"]

	return scalarMemoryUsageOk || scalarMemoryUsedOk || scalarMemoryTotalOk || scalarMemoryFreeOk
}

func (ms *MetricSender) trySendScalarMemoryUsage(scalarSamples map[string]MetricSample) error {
	_, scalarMemoryUsageOk := scalarSamples["memory.usage"]
	if scalarMemoryUsageOk {
		// memory usage is already sent through collected metrics
		return nil
	}

	scalarMemoryUsed, scalarMemoryUsedOk := scalarSamples["memory.used"]
	scalarMemoryTotal, scalarMemoryTotalOk := scalarSamples["memory.total"]
	scalarMemoryFree, scalarMemoryFreeOk := scalarSamples["memory.free"]

	if scalarMemoryUsedOk {
		floatMemoryUsed, err := scalarMemoryUsed.value.ToFloat64()
		if err != nil {
			return fmt.Errorf("metric `%s`: failed to convert to float64: %s", "memory.used", err)
		}

		if scalarMemoryTotalOk {
			// memory total and memory used
			floatMemoryTotal, err := scalarMemoryTotal.value.ToFloat64()
			if err != nil {
				return fmt.Errorf("metric `%s`: failed to convert to float64: %s", "memory.total", err)
			}

			memoryUsageValue, err := evaluateMemoryUsage(floatMemoryUsed, floatMemoryTotal)
			if err != nil {
				return err
			}

			memoryUsageSample := MetricSample{
				value:      valuestore.ResultValue{Value: memoryUsageValue},
				tags:       scalarMemoryUsed.tags,
				symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
				options:    profiledefinition.MetricsConfigOption{},
				forcedType: "",
			}

			ms.sendMetric(memoryUsageSample)
			return nil
		}

		if scalarMemoryFreeOk {
			// memory total and memory used
			floatMemoryFree, err := scalarMemoryFree.value.ToFloat64()
			if err != nil {
				log.Debugf("metric `%s`: failed to convert to float64: %s", "memory.free", err)
				return err
			}

			floatMemoryUsed, err := scalarMemoryUsed.value.ToFloat64()
			if err != nil {
				log.Debugf("metric `%s`: failed to convert to float64: %s", "memory.used", err)
				return err
			}

			memoryUsageValue, err := evaluateMemoryUsage(floatMemoryUsed, floatMemoryFree+floatMemoryUsed)
			if err != nil {
				return err
			}

			memoryUsageSample := MetricSample{
				value:      valuestore.ResultValue{Value: memoryUsageValue},
				tags:       scalarMemoryUsed.tags,
				symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
				options:    profiledefinition.MetricsConfigOption{},
				forcedType: "",
			}

			ms.sendMetric(memoryUsageSample)
			return nil
		}
	}

	if scalarMemoryFreeOk && scalarMemoryTotalOk {
		// memory total and memory used
		floatMemoryFree, err := scalarMemoryFree.value.ToFloat64()
		if err != nil {
			log.Debugf("metric `%s`: failed to convert to float64: %s", "memory.free", err)
			return err
		}

		floatMemoryTotal, err := scalarMemoryTotal.value.ToFloat64()
		if err != nil {
			log.Debugf("metric `%s`: failed to convert to float64: %s", "memory.total", err)
			return err
		}

		memoryUsageValue, err := evaluateMemoryUsage(floatMemoryTotal-floatMemoryFree, floatMemoryTotal)
		if err != nil {
			return err
		}

		memoryUsageSample := MetricSample{
			value:      valuestore.ResultValue{Value: memoryUsageValue},
			tags:       scalarMemoryTotal.tags,
			symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
			options:    profiledefinition.MetricsConfigOption{},
			forcedType: "",
		}

		ms.sendMetric(memoryUsageSample)
		return nil
	}

	// report missing dependency metrics
	missingMetrics := []string{}
	if !scalarMemoryUsedOk {
		missingMetrics = append(missingMetrics, "used")
	}
	if !scalarMemoryFreeOk {
		missingMetrics = append(missingMetrics, "free")
	}
	if !scalarMemoryTotalOk {
		missingMetrics = append(missingMetrics, "total")
	}

	return fmt.Errorf("missing %s memory metrics, skipping scalar memory usage", strings.Join(missingMetrics, ", "))
}

func (ms *MetricSender) trySendColumnMemoryUsage(columnSamples map[string]map[string]MetricSample) error {
	_, memoryUsageOk := columnSamples["memory.usage"]
	if memoryUsageOk {
		// memory usage is already sent through collected metrics
		return nil
	}

	memoryUsedRows, memoryUsedOk := columnSamples["memory.used"]
	memoryTotalRows, memoryTotalOk := columnSamples["memory.total"]
	memoryFreeRows, memoryFreeOk := columnSamples["memory.free"]

	if memoryUsedOk {
		if memoryTotalOk {
			for rowIndex, memoryUsedSample := range memoryUsedRows {
				memoryTotalSample, memoryTotalSampleOk := memoryTotalRows[rowIndex]
				if !memoryTotalSampleOk {
					return fmt.Errorf("missing memory total sample at row %s, skipping memory usage evaluation", rowIndex)
				}
				floatMemoryTotal, err := memoryTotalSample.value.ToFloat64()
				if err != nil {
					return fmt.Errorf("metric `%s[%s]`: failed to convert to float64: %s", "memory.total", rowIndex, err)
				}

				floatMemoryUsed, err := memoryUsedSample.value.ToFloat64()
				if err != nil {
					return fmt.Errorf("metric `%s[%s]`: failed to convert to float64: %s", "memory.used", rowIndex, err)
				}

				memoryUsageValue, err := evaluateMemoryUsage(floatMemoryUsed, floatMemoryTotal)
				if err != nil {
					return err
				}

				memoryUsageSample := MetricSample{
					value:      valuestore.ResultValue{Value: memoryUsageValue},
					tags:       memoryUsedSample.tags,
					symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				}

				ms.sendMetric(memoryUsageSample)
			}
			return nil
		}

		if memoryFreeOk {
			for rowIndex, memoryUsedSample := range memoryUsedRows {
				memoryFreeSample, memoryFreeSampleOk := memoryFreeRows[rowIndex]
				if !memoryFreeSampleOk {
					return fmt.Errorf("missing memory free sample at row %s, skipping memory usage evaluation", rowIndex)
				}
				floatMemoryFree, err := memoryFreeSample.value.ToFloat64()
				if err != nil {
					return fmt.Errorf("metric `%s[%s]`: failed to convert to float64: %s", "memory.free", rowIndex, err)
				}

				floatMemoryUsed, err := memoryUsedSample.value.ToFloat64()
				if err != nil {
					return fmt.Errorf("metric `%s[%s]`: failed to convert to float64: %s", "memory.used", rowIndex, err)
				}

				memoryUsageValue, err := evaluateMemoryUsage(floatMemoryUsed, floatMemoryFree+floatMemoryUsed)
				if err != nil {
					return err
				}

				memoryUsageSample := MetricSample{
					value:      valuestore.ResultValue{Value: memoryUsageValue},
					tags:       memoryUsedSample.tags,
					symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
					options:    profiledefinition.MetricsConfigOption{},
					forcedType: "",
				}

				ms.sendMetric(memoryUsageSample)
			}
			return nil
		}
	}

	if memoryFreeOk && memoryTotalOk {
		for rowIndex, memoryTotalSample := range memoryTotalRows {
			memoryFreeSample, memoryFreeSampleOk := memoryFreeRows[rowIndex]
			if !memoryFreeSampleOk {
				return fmt.Errorf("missing memory free sample at row %s, skipping memory usage evaluation", rowIndex)
			}
			floatMemoryFree, err := memoryFreeSample.value.ToFloat64()
			if err != nil {
				return fmt.Errorf("metric `%s[%s]`: failed to convert to float64: %s", "memory.free", rowIndex, err)
			}

			floatMemoryTotal, err := memoryTotalSample.value.ToFloat64()
			if err != nil {
				return fmt.Errorf("metric `%s[%s]`: failed to convert to float64: %s", "memory.total", rowIndex, err)
			}

			memoryUsageValue, err := evaluateMemoryUsage(floatMemoryTotal-floatMemoryFree, floatMemoryTotal)
			if err != nil {
				return err
			}

			memoryUsageSample := MetricSample{
				value:      valuestore.ResultValue{Value: memoryUsageValue},
				tags:       memoryTotalSample.tags,
				symbol:     profiledefinition.SymbolConfig{Name: "memory.usage"},
				options:    profiledefinition.MetricsConfigOption{},
				forcedType: "",
			}

			ms.sendMetric(memoryUsageSample)
		}
		return nil
	}

	// report missing dependency metrics
	missingMetrics := []string{}
	if !memoryUsedOk {
		missingMetrics = append(missingMetrics, "used")
	}
	if !memoryFreeOk {
		missingMetrics = append(missingMetrics, "free")
	}
	if !memoryTotalOk {
		missingMetrics = append(missingMetrics, "total")
	}

	return fmt.Errorf("missing %s memory metrics, skipping column memory usage", strings.Join(missingMetrics, ", "))
}

func evaluateMemoryUsage(memoryUsed float64, memoryTotal float64) (float64, error) {
	if memoryTotal == 0 {
		return 0, fmt.Errorf("cannot evaluate memory usage, total memory is 0")
	}
	if memoryUsed < 0 {
		return 0, fmt.Errorf("cannot evaluate memory usage, memory used is < 0")
	}
	return (memoryUsed / memoryTotal) * 100, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package observerimpl

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/google/pprof/profile"

	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
)

type AnomalyDetection struct {
	log logger.Component
}

func NewAnomalyDetection(log logger.Component) *AnomalyDetection {
	return &AnomalyDetection{
		log: log,
	}
}

func (a *AnomalyDetection) ProcessMetric(metric *metricObs) {
	if !strings.HasPrefix(metric.name, "datadog") {
		a.log.Debugf("Processing metric: %v", metric)
	}
}

func (a *AnomalyDetection) ProcessLog(log *logObs) {
	a.log.Debugf("Processing log: %v", log)
}

func (a *AnomalyDetection) ProcessTrace(trace *traceObs) {
	a.log.Debugf("Processing trace: %v", trace)
}

func (a *AnomalyDetection) ProcessProfile(profile *profileObs) {
	if profile.profileType == "go" {
		// Get the 10 top most CPU consuming functions from the go pprof (store in rawData)
		topFunctions, err := a.getTopCPUFunctions(profile.rawData, 10)
		if err != nil {
			a.log.Warnf("Failed to parse pprof data: %v", err)
		} else {
			a.log.Infof("Top 10 CPU consuming functions:")
			for i, fn := range topFunctions {
				a.log.Infof("  %d. %s: %.2f%% (flat: %d, cum: %d)",
					i+1, fn.Name, fn.Percentage, fn.Flat, fn.Cumulative)
			}
		}
	}
	a.log.Debugf("Processing profile: %v", profile)
}

type CPUFunction struct {
	Name       string
	Flat       int64
	Cumulative int64
	Percentage float64
}

func (a *AnomalyDetection) getTopCPUFunctions(rawData []byte, topN int) ([]CPUFunction, error) {
	// Extract CPU profile from multipart form data
	cpuProfileData, err := a.extractCPUProfile(rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to extract CPU profile: %w", err)
	}

	if cpuProfileData == nil {
		return nil, fmt.Errorf("no CPU profile found in the data")
	}

	// Parse the pprof data
	prof, err := profile.Parse(bytes.NewReader(cpuProfileData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}

	// Calculate the total value for percentage calculations
	var totalValue int64
	for _, sample := range prof.Sample {
		if len(sample.Value) > 0 {
			totalValue += sample.Value[0]
		}
	}

	// Aggregate samples by function name
	functionStats := make(map[string]*CPUFunction)

	for _, sample := range prof.Sample {
		if len(sample.Value) == 0 {
			continue
		}

		flatValue := sample.Value[0]

		// Get the function at the top of the stack (leaf function)
		if len(sample.Location) > 0 && len(sample.Location[0].Line) > 0 {
			fn := sample.Location[0].Line[0].Function
			if fn != nil {
				funcName := fn.Name
				if stats, exists := functionStats[funcName]; exists {
					stats.Flat += flatValue
				} else {
					functionStats[funcName] = &CPUFunction{
						Name: funcName,
						Flat: flatValue,
					}
				}
			}
		}

		// Add cumulative values for all functions in the stack
		for _, location := range sample.Location {
			for _, line := range location.Line {
				if line.Function != nil {
					funcName := line.Function.Name
					if stats, exists := functionStats[funcName]; exists {
						stats.Cumulative += flatValue
					} else if _, exists := functionStats[funcName]; !exists {
						functionStats[funcName] = &CPUFunction{
							Name:       funcName,
							Flat:       0,
							Cumulative: flatValue,
						}
					}
				}
			}
		}
	}

	// Convert map to slice and calculate percentages
	functions := make([]CPUFunction, 0, len(functionStats))
	for _, stats := range functionStats {
		if totalValue > 0 {
			stats.Percentage = (float64(stats.Flat) / float64(totalValue)) * 100
		}
		functions = append(functions, *stats)
	}

	// Sort by flat value (descending)
	for i := 0; i < len(functions); i++ {
		for j := i + 1; j < len(functions); j++ {
			if functions[j].Flat > functions[i].Flat {
				functions[i], functions[j] = functions[j], functions[i]
			}
		}
	}

	// Return top N functions
	if len(functions) > topN {
		functions = functions[:topN]
	}

	return functions, nil
}

func (a *AnomalyDetection) extractCPUProfile(rawData []byte) ([]byte, error) {
	// Try to parse as multipart form data
	// First, we need to extract the boundary from the data
	boundary, err := a.extractBoundary(rawData)
	if err != nil {
		// If we can't extract the boundary, assume it's raw pprof data
		return rawData, nil
	}

	// Create a multipart reader with the extracted boundary
	reader := multipart.NewReader(bytes.NewReader(rawData), boundary)

	// Iterate through parts to find the CPU profile
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading multipart data: %w", err)
		}

		// Check if this part is a CPU profile
		// Common names: "cpu.pprof", "delta-prof.pprof", "profile.pprof"
		filename := part.FileName()
		a.log.Debugf("Found multipart file: %s", filename)

		if strings.Contains(filename, "cpu") ||
			strings.Contains(filename, "prof.pprof") ||
			filename == "profile.pprof" {
			// Read the profile data
			data, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				return nil, fmt.Errorf("error reading CPU profile part: %w", err)
			}
			a.log.Debugf("Extracted CPU profile from %s (%d bytes)", filename, len(data))
			return data, nil
		}
		part.Close()
	}

	return nil, nil
}

func (a *AnomalyDetection) extractBoundary(rawData []byte) (string, error) {
	// Look for the first boundary in the data
	// Multipart data starts with: --boundary
	lines := bytes.Split(rawData, []byte("\n"))
	if len(lines) == 0 {
		return "", fmt.Errorf("empty data")
	}

	firstLine := string(bytes.TrimSpace(lines[0]))
	if !strings.HasPrefix(firstLine, "--") {
		return "", fmt.Errorf("data doesn't start with boundary")
	}

	// Extract boundary (remove the leading --)
	boundary := strings.TrimPrefix(firstLine, "--")
	if boundary == "" {
		return "", fmt.Errorf("empty boundary")
	}

	return boundary, nil
}

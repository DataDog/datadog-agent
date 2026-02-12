// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomaly

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"sort"
	"strings"

	"github.com/google/pprof/profile"
)

// ProfileProcessor handles processing of multiple profile data
type ProfileProcessor struct {
	// Add any configuration or dependencies here if needed
}

// NewProfileProcessor creates a new ProfileProcessor instance
func NewProfileProcessor() *ProfileProcessor {
	return &ProfileProcessor{}
}

// CumulativeProfileData represents the aggregated CPU and memory profile data
type CumulativeProfileData struct {
	// CPU profile data aggregated across all profiles
	CPUStats map[string]int64 // function name -> cumulative sample count

	// Memory profile data aggregated across all profiles
	MemoryStats map[string]int64 // function name -> cumulative allocated bytes
}

// CPUFunction represents a function with its CPU sample count
type CPUFunction struct {
	Name string
	Flat int64
}

// MemoryFunction represents a function with its memory allocation
type MemoryFunction struct {
	Name  string
	Bytes int64
}

// TopFunctions contains top CPU and memory consuming functions
type TopFunctions struct {
	CPU    []CPUFunction
	Memory []MemoryFunction
}

// ProcessProfiles takes a list of raw profile data and returns cumulative CPU and memory statistics
func (p *ProfileProcessor) ProcessProfiles(rawDataList [][]byte) (*CumulativeProfileData, error) {
	result := &CumulativeProfileData{
		CPUStats:    make(map[string]int64),
		MemoryStats: make(map[string]int64),
	}

	for i, rawData := range rawDataList {
		// Extract CPU profile if present
		cpuData, err := p.extractCPUProfile(rawData)
		if err == nil && cpuData != nil {
			if err := p.aggregateCPUProfile(cpuData, result.CPUStats); err != nil {
				return nil, fmt.Errorf("failed to aggregate CPU profile %d: %w", i, err)
			}
		}

		// Extract memory profile if present
		memoryData, err := p.extractMemoryProfile(rawData)
		if err == nil && memoryData != nil {
			if err := p.aggregateMemoryProfile(memoryData, result.MemoryStats); err != nil {
				return nil, fmt.Errorf("failed to aggregate memory profile %d: %w", i, err)
			}
		}
	}

	return result, nil
}

// GetTopFunctions processes a list of raw profile data and returns the top N CPU and memory consuming functions
func (p *ProfileProcessor) GetTopFunctions(rawDataList [][]byte, topN int) (TopFunctions, error) {
	cumulativeData, err := p.ProcessProfiles(rawDataList)
	if err != nil {
		return TopFunctions{}, fmt.Errorf("failed to process profiles: %w", err)
	}

	return TopFunctions{
		CPU: topNFromStats(cumulativeData.CPUStats, topN, func(name string, value int64) CPUFunction {
			return CPUFunction{Name: name, Flat: value}
		}),
		Memory: topNFromStats(cumulativeData.MemoryStats, topN, func(name string, value int64) MemoryFunction {
			return MemoryFunction{Name: name, Bytes: value}
		}),
	}, nil
}

// itemWithValue pairs an item with its value for sorting
type itemWithValue[T any] struct {
	item  T
	value int64
}

// topNFromStats is a generic helper to get top N items from a stats map
func topNFromStats[T any](stats map[string]int64, topN int, builder func(string, int64) T) []T {
	if len(stats) == 0 {
		return nil
	}

	// Convert map to slice of pairs
	pairs := make([]itemWithValue[T], 0, len(stats))
	for name, value := range stats {
		pairs = append(pairs, itemWithValue[T]{
			item:  builder(name, value),
			value: value,
		})
	}

	// Sort by value (descending)
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].value > pairs[j].value
	})

	// Extract items from sorted pairs
	items := make([]T, 0, len(pairs))
	for _, pair := range pairs {
		items = append(items, pair.item)
	}

	// Return top N
	if len(items) > topN {
		return items[:topN]
	}
	return items
}

// profileMatcher is a function that determines if a filename matches a profile type
type profileMatcher func(filename string) bool

// profileExtractor handles extraction of profiles from multipart data
type profileExtractor struct {
	profileType    string
	matcher        profileMatcher
	allowRawPprof  bool
	errorIfMissing bool
}

// extractProfile is a factory-based generic extraction function for profiles
func (p *ProfileProcessor) extractProfile(rawData []byte, extractor profileExtractor) ([]byte, error) {
	// Try to parse as multipart form data
	boundary, err := extractBoundary(rawData)
	if err != nil {
		// If we can't extract the boundary, check if we should treat as raw pprof
		if extractor.allowRawPprof {
			// Try to parse it directly to verify
			if _, parseErr := profile.Parse(bytes.NewReader(rawData)); parseErr == nil {
				return rawData, nil
			}
			if extractor.errorIfMissing {
				return nil, fmt.Errorf("not multipart data and not valid pprof: %w", err)
			}
		}
		// Not an error for profiles that don't support raw format
		return nil, nil
	}

	// Create a multipart reader with the extracted boundary
	reader := multipart.NewReader(bytes.NewReader(rawData), boundary)

	// Iterate through parts to find the profile
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading multipart data: %w", err)
		}

		filename := part.FileName()

		// Check if this part matches the profile type we're looking for
		if extractor.matcher(filename) {
			data, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				return nil, fmt.Errorf("error reading %s profile part: %w", extractor.profileType, err)
			}
			return data, nil
		}
		part.Close()
	}

	return nil, nil
}

// cpuProfileMatcher checks if a filename is a CPU profile
func cpuProfileMatcher(filename string) bool {
	return strings.Contains(filename, "cpu") ||
		strings.Contains(filename, "prof.pprof") ||
		filename == "profile.pprof"
}

// memoryProfileMatcher checks if a filename is a memory/heap profile
func memoryProfileMatcher(filename string) bool {
	return strings.Contains(filename, "heap") ||
		strings.Contains(filename, "memory") ||
		strings.Contains(filename, "alloc")
}

// extractCPUProfile extracts CPU profile data from raw multipart or pprof data
func (p *ProfileProcessor) extractCPUProfile(rawData []byte) ([]byte, error) {
	extractor := profileExtractor{
		profileType:    "CPU",
		matcher:        cpuProfileMatcher,
		allowRawPprof:  true,
		errorIfMissing: true,
	}
	return p.extractProfile(rawData, extractor)
}

// extractMemoryProfile extracts memory profile data from raw multipart or pprof data
func (p *ProfileProcessor) extractMemoryProfile(rawData []byte) ([]byte, error) {
	extractor := profileExtractor{
		profileType:    "memory",
		matcher:        memoryProfileMatcher,
		allowRawPprof:  false,
		errorIfMissing: false,
	}
	return p.extractProfile(rawData, extractor)
}

// extractBoundary extracts the multipart boundary from raw data
func extractBoundary(rawData []byte) (string, error) {
	// Look for the first boundary in the data
	// Multipart data starts with: --boundary
	lines := bytes.Split(rawData, []byte("\n"))
	if len(lines) == 0 {
		return "", errors.New("empty data")
	}

	firstLine := string(bytes.TrimSpace(lines[0]))
	if !strings.HasPrefix(firstLine, "--") {
		return "", errors.New("data doesn't start with boundary")
	}

	// Extract boundary (remove the leading --)
	boundary := strings.TrimPrefix(firstLine, "--")
	if boundary == "" {
		return "", errors.New("empty boundary")
	}

	return boundary, nil
}

// aggregateCPUProfile parses a CPU profile and aggregates its data into the stats map
func (p *ProfileProcessor) aggregateCPUProfile(profileData []byte, stats map[string]int64) error {
	prof, err := profile.Parse(bytes.NewReader(profileData))
	if err != nil {
		return fmt.Errorf("failed to parse CPU profile: %w", err)
	}

	// Aggregate samples by function name
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
				stats[funcName] += flatValue
			}
		}
	}

	return nil
}

// aggregateMemoryProfile parses a memory profile and aggregates its data into the stats map
func (p *ProfileProcessor) aggregateMemoryProfile(profileData []byte, stats map[string]int64) error {
	prof, err := profile.Parse(bytes.NewReader(profileData))
	if err != nil {
		return fmt.Errorf("failed to parse memory profile: %w", err)
	}

	// For memory profiles, we typically look at allocated bytes (alloc_space or inuse_space)
	// The index varies, but commonly:
	// - Value[0]: alloc_objects (number of objects allocated)
	// - Value[1]: alloc_space (bytes allocated)
	// - Value[2]: inuse_objects (number of objects currently in use)
	// - Value[3]: inuse_space (bytes currently in use)

	for _, sample := range prof.Sample {
		if len(sample.Value) < 2 {
			continue
		}

		// Use alloc_space (bytes allocated) as the primary metric
		allocBytes := sample.Value[1]

		// Get the function that allocated the memory
		if len(sample.Location) > 0 && len(sample.Location[0].Line) > 0 {
			fn := sample.Location[0].Line[0].Function
			if fn != nil {
				funcName := fn.Name
				stats[funcName] += allocBytes
			}
		}
	}

	return nil
}

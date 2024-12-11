// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package lutgen provides tools to generate lookup tables for Go binaries.
package lutgen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/go/goversion"
)

// LookupFunction configures a single lookup table
// as a Go function that maps an input Go version/architecture
// to the specified output type.
type LookupFunction struct {
	Name            string
	OutputType      string
	OutputZeroValue string
	ExtractValue    func(inspectionResult interface{}) interface{}
	RenderValue     func(value interface{}) string
	DocComment      string
}

// argsFromResultTable converts the "result table"
// (map of architecture,version pairs to inspection results)
// to the template arguments, ready to render the lookup table implementation.
// To do this, it:
//   - converts each result to the specific "value" for the function
//     by running the "ExtractValue" function, if given.
//     If used, this lets a single run of the matrix runner
//     generate multiple lookup functions, each working with different values
//     that get derived from the inspection result values.
//   - bin the results by architecture,
//     since the lookup tables are generated with a top-level "switch...case"
//     on the architectures.
//   - for each architecture, sort the values by the Go version
//     and drop any non-unique values.
//     Then, reverse the list of Go version, value pairs
//     to generate the list of cases used to render the lookup table.
//     This works because the lookup table generates a list of
//     "if $inputVersion > $caseVersion { return $caseValue }" statements for each case,
//     so by reverse-sorting the list of cases, the appropriate case will always be taken.
//     This has the added benefit of gracefully handling unknown/newer Go versions
//     than the lookup table was generated with; they will take the first branch.
func (f *LookupFunction) argsFromResultTable(resultTable map[architectureVersion]interface{}) lookupFunctionTemplateArgs {
	valueTable := f.convertResultToValueTable(resultTable)
	architectureValueSets := f.binArchitectures(valueTable)

	// Sort the list of architecture cases so that the output is deterinstic
	sort.Slice(architectureValueSets, func(i, j int) bool {
		return architectureValueSets[i].arch < architectureValueSets[j].arch
	})

	// For each architecture case,
	// prepare its template args by compressing the lookup tables to unique values.
	archCaseArgs := []archCaseTemplateArgs{}
	for _, valueSet := range architectureValueSets {
		compressedValueSet := f.compressLookupTable(valueSet)
		archCaseArgs = append(archCaseArgs, f.prepareArchitectureCase(compressedValueSet))
	}

	// Add double-slashes to each line in the doc comment
	docComment := f.DocComment
	if len(docComment) > 0 {
		docCommentLines := strings.Split(docComment, "\n")
		var sb strings.Builder
		for _, line := range docCommentLines {
			sb.WriteString(fmt.Sprintf("// %s\n", line))
		}
		docComment = sb.String()
	}

	return lookupFunctionTemplateArgs{
		Name:               f.Name,
		OutputType:         f.OutputType,
		OutputZeroValue:    f.OutputZeroValue,
		RenderedDocComment: docComment,
		ArchCases:          archCaseArgs,
	}
}

func (f *LookupFunction) convertResultToValueTable(resultTable map[architectureVersion]interface{}) map[architectureVersion]interface{} {
	// Run the extraction function on each element,
	// or if none was given, return the same table.
	if f.ExtractValue == nil {
		return resultTable
	}

	valueTable := make(map[architectureVersion]interface{})
	for av, result := range resultTable {
		valueTable[av] = f.ExtractValue(result)
	}

	return valueTable
}

type architectureValueSet struct {
	arch   string
	values map[goversion.GoVersion]interface{}
}

func (f *LookupFunction) binArchitectures(valueTable map[architectureVersion]interface{}) []architectureValueSet {
	// Group the value table by the architecture,
	// storing all values in architecture-specific sub-maps.
	architectureBins := make(map[string]map[goversion.GoVersion]interface{})
	for av, value := range valueTable {
		curr := architectureBins[av.architecture]
		if curr == nil {
			curr = make(map[goversion.GoVersion]interface{})
		}
		curr[av.version] = value
		architectureBins[av.architecture] = curr
	}

	// Convert the sub-maps to slices
	valueSets := []architectureValueSet{}
	for arch, values := range architectureBins {
		valueSets = append(valueSets, architectureValueSet{
			arch:   arch,
			values: values,
		})
	}

	return valueSets
}

type compressedArchitectureValueSet struct {
	arch      string
	intervals []entry
}

type entry struct {
	version goversion.GoVersion
	value   interface{}
}

func (f *LookupFunction) compressLookupTable(valueSet architectureValueSet) compressedArchitectureValueSet {
	entries := make([]entry, len(valueSet.values))
	i := 0
	for v, value := range valueSet.values {
		entries[i] = entry{
			value:   value,
			version: v,
		}
		i++
	}

	// Sort the entries in order of increasing go version
	sort.Slice(entries, func(x, y int) bool {
		return !entries[x].version.AfterOrEqual(entries[y].version)
	})

	// Eliminate contiguous same values
	hasCurrentValue := false
	var currentValue interface{}
	intervals := []entry{}
	for _, entry := range entries {
		if !hasCurrentValue || !reflect.DeepEqual(currentValue, entry.value) {
			hasCurrentValue = true
			currentValue = entry.value
			intervals = append(intervals, entry)
		}
	}

	// Sort the intervals in order of decreasing go version
	sort.Slice(intervals, func(x, y int) bool {
		return intervals[x].version.AfterOrEqual(intervals[y].version)
	})

	return compressedArchitectureValueSet{
		arch:      valueSet.arch,
		intervals: intervals,
	}
}

func (f *LookupFunction) prepareArchitectureCase(compressedValueSet compressedArchitectureValueSet) archCaseTemplateArgs {
	// Find the min go version, if any
	var minVersion goversion.GoVersion
	hasMin := len(compressedValueSet.intervals) > 0
	if hasMin {
		minVersion = compressedValueSet.intervals[len(compressedValueSet.intervals)-1].version
	}

	// Render the value in each interval/branch to produce the branch template args
	branchArgs := []branchTemplateArgs{}
	for _, interval := range compressedValueSet.intervals {
		var renderedValue string
		if f.RenderValue != nil {
			renderedValue = f.RenderValue(interval.value)
		} else {
			// Use the "Go syntax" formatting verb.
			// This usually works pretty well.
			renderedValue = fmt.Sprintf("%#v", interval.value)
		}

		branchArgs = append(branchArgs, branchTemplateArgs{
			Version:       interval.version,
			RenderedValue: renderedValue,
		})
	}

	return archCaseTemplateArgs{
		Arch:     compressedValueSet.arch,
		HasMin:   hasMin,
		Min:      minVersion,
		Branches: branchArgs,
	}
}

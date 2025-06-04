// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package msi contains helper functions to work with msi packages
package msi

import (
	"bytes"
	"fmt"
	"golang.org/x/text/encoding/unicode"
	"io"
	"io/fs"
	"regexp"
	"sort"
)

// TextRange is a simple struct to represent a range of text in a file.
type TextRange struct {
	start int
	end   int
}

// FindAllIndexWithContext is similar to FindAllIndex but expands the matched range for a number of lines
// before and after the TextRange (called contextBefore and contextAfter).
func FindAllIndexWithContext(r *regexp.Regexp, input []byte, contextBefore, contextAfter int) []TextRange {
	contextBefore = max(contextBefore, 0)
	contextAfter = max(contextAfter, 0)
	var extractedRanges []TextRange
	results := r.FindAllIndex(input, -1)
	for _, result := range results {
		lineCounter := 0
		charCounter := result[0]
		for ; charCounter >= 0; charCounter-- {
			if input[charCounter] == '\n' {
				lineCounter++
			}
			if lineCounter > contextBefore {
				break
			}
		}
		lineStart := charCounter + 1

		lineCounter = 0
		charCounter = result[1]
		for ; charCounter < len(input); charCounter++ {
			if input[charCounter] == '\n' {
				lineCounter++
			}
			if lineCounter > contextAfter {
				break
			}
		}
		lineEnd := charCounter

		extractedRanges = append(extractedRanges, TextRange{lineStart, lineEnd})
	}

	return extractedRanges
}

// insert merges newRanges into existingRanges by combining overlapping or adjacent ranges.
func insert(existingRanges, newRanges []TextRange) []TextRange {
	// Combine all ranges into a single slice for sorting
	allRanges := append(existingRanges, newRanges...)

	// Sort ranges by start value (and end value if starts are equal)
	sort.Slice(allRanges, func(i, j int) bool {
		if allRanges[i].start == allRanges[j].start {
			return allRanges[i].end < allRanges[j].end
		}
		return allRanges[i].start < allRanges[j].start
	})

	// Merge ranges
	var merged []TextRange
	for _, current := range allRanges {
		// If merged is empty or the current range does not overlap with the last merged range
		if len(merged) == 0 || merged[len(merged)-1].end < current.start {
			merged = append(merged, current) // Add the current range
		} else {
			// Overlapping or adjacent: Extend the end of the last merged range
			merged[len(merged)-1].end = max(merged[len(merged)-1].end, current.end)
		}
	}

	return merged
}

// Combine processes input using multiple logFileProcessors and merges their output ranges.
func Combine(input []byte, processors ...logFileProcessor) []TextRange {
	var allRanges []TextRange

	// Collect all ranges from each processor
	for _, processor := range processors {
		allRanges = append(allRanges, processor(input)...)
	}

	// Use the improved insert function to merge all collected ranges
	return insert(nil, allRanges)
}

type logFileProcessor func([]byte) []TextRange

// processLogFile reads a UTF-16 MSI log file and applies various processors on it
// to retain only the relevant log lines. It combines the various outputs from the processors and
// decorate each range of log lines with a marker ('---') to distinguish them.
func processLogFile(logFile fs.File, processors ...logFileProcessor) ([]byte, error) {
	logFileBuffer := bytes.NewBuffer(nil)
	_, err := io.Copy(logFileBuffer, logFile)
	if err != nil {
		return nil, err
	}
	decodedLogsBytes, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Bytes(logFileBuffer.Bytes())
	if err != nil {
		return nil, err
	}

	var output []byte
	rangesToSave := Combine(decodedLogsBytes, processors...)
	for _, ranges := range rangesToSave {
		output = append(output, []byte(fmt.Sprintf("--- %d:%d\r\n", ranges.start, ranges.end))...)
		output = append(output, decodedLogsBytes[ranges.start:ranges.end]...)
		output = append(output, '\r', '\n', '\r', '\n')
	}
	return output, nil
}

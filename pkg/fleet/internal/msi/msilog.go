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
)

// FindAllIndexWithContext is similar to FindAllIndex but expands the matched range for a number of lines
// before and after the match (called contextBefore and contextAfter).
func FindAllIndexWithContext(r *regexp.Regexp, input []byte, contextBefore, contextAfter int) [][]int {
	var extractedRanges [][]int
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

		extractedRanges = append(extractedRanges, []int{lineStart, lineEnd})
	}

	return extractedRanges
}

func insert(existingRanges, newRanges [][]int) [][]int {
	for _, newRange := range newRanges {
		matched := false
		for _, existingRange := range existingRanges {
			bnr := newRange[0]
			enr := newRange[1]
			ber := existingRange[0]
			eer := existingRange[1]
			// [ber, eer]
			//   [bnr,  enr]
			if bnr <= eer && eer < enr {
				existingRange[1] = newRange[1]
				matched = true
			}
			//   [ber, eer]
			// [bnr,  enr]
			if bnr <= ber && ber < enr {
				existingRange[0] = newRange[0]
				matched = true
			}
			//   [ber, eer]
			// [bnr,      enr]
			if bnr <= ber && enr >= eer {
				existingRange[0] = newRange[0]
				existingRange[1] = newRange[1]
				matched = true
			}
			// [ber,    eer]
			//   [bnr, enr]
			if ber <= bnr && enr <= eer {
				matched = true
			}
		}
		if !matched {
			existingRanges = append(existingRanges, newRange)
		}
	}
	return existingRanges
}

// Combine combines the output of multiple logFileProcessors
func Combine(input []byte, processors ...logFileProcessor) [][]int {
	var ranges [][]int
	for _, processor := range processors {
		r := processor(input)
		if ranges == nil {
			ranges = append(ranges, r[0])
			if len(r) > 1 {
				ranges = insert(ranges, r[1:])
			}
		} else {
			ranges = insert(ranges, r)
		}
	}
	return ranges
}

type logFileProcessor func([]byte) [][]int

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
		output = append(output, []byte(fmt.Sprintf("--- %d:%d\r\n", ranges[0], ranges[1]))...)
		output = append(output, decodedLogsBytes[ranges[0]:ranges[1]]...)
		output = append(output, '\r', '\n', '\r', '\n')
	}
	return output, nil
}

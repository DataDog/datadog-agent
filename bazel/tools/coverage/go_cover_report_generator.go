// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/cover"
)

func mergeBlock(profile *cover.Profile, block cover.ProfileBlock) error {
	index := sort.Search(len(profile.Blocks), func(i int) bool {
		current := profile.Blocks[i]
		return current.StartLine > block.StartLine ||
			(current.StartLine == block.StartLine && current.StartCol >= block.StartCol)
	})

	if index < len(profile.Blocks) &&
		profile.Blocks[index].StartLine == block.StartLine &&
		profile.Blocks[index].StartCol == block.StartCol {
		current := &profile.Blocks[index]
		if current.EndLine != block.EndLine || current.EndCol != block.EndCol || current.NumStmt != block.NumStmt {
			return fmt.Errorf("incompatible coverage blocks in %s at %d.%d", profile.FileName, block.StartLine, block.StartCol)
		}
		switch profile.Mode {
		case "set":
			current.Count |= block.Count
		case "count", "atomic":
			current.Count += block.Count
		default:
			return fmt.Errorf("unsupported coverage mode %q", profile.Mode)
		}
		return nil
	}

	profile.Blocks = append(profile.Blocks, cover.ProfileBlock{})
	copy(profile.Blocks[index+1:], profile.Blocks[index:])
	profile.Blocks[index] = block
	return nil
}

func mergeProfiles(profilesByFile map[string]*cover.Profile, profiles []*cover.Profile, mode *string) error {
	for _, profile := range profiles {
		if *mode == "" {
			*mode = profile.Mode
		} else if profile.Mode != *mode {
			return fmt.Errorf("cannot merge coverage modes %q and %q", *mode, profile.Mode)
		}

		merged, found := profilesByFile[profile.FileName]
		if !found {
			profilesByFile[profile.FileName] = profile
			continue
		}
		for _, block := range profile.Blocks {
			if err := mergeBlock(merged, block); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergeBaselineProfiles(profilesByFile map[string]*cover.Profile, profiles []*cover.Profile, mode *string) error {
	for _, profile := range profiles {
		if *mode == "" {
			*mode = profile.Mode
		} else if profile.Mode != *mode {
			return fmt.Errorf("cannot merge coverage modes %q and %q", *mode, profile.Mode)
		}
		if _, found := profilesByFile[profile.FileName]; found {
			continue
		}
		profilesByFile[profile.FileName] = profile
	}
	return nil
}

func readReportPaths(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if path := strings.TrimSpace(scanner.Text()); path != "" {
			paths = append(paths, path)
		}
	}
	return paths, scanner.Err()
}

func parseGoProfiles(path string) ([]*cover.Profile, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	scanner := bufio.NewScanner(file)
	isGoProfile := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			isGoProfile = strings.HasPrefix(line, "mode: ")
			break
		}
	}
	if err := scanner.Err(); err != nil {
		file.Close()
		return nil, false, err
	}
	if err := file.Close(); err != nil {
		return nil, false, err
	}
	if !isGoProfile {
		return nil, false, nil
	}

	profiles, err := cover.ParseProfiles(path)
	return profiles, true, err
}

func zeroBlocksForBaselineFile(fileName string) ([]cover.ProfileBlock, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return []cover.ProfileBlock{{
			StartLine: 1,
			StartCol:  0,
			EndLine:   1,
			EndCol:    1,
			NumStmt:   1,
			Count:     0,
		}}, nil
	}
	defer file.Close()

	var blocks []cover.ProfileBlock
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		blocks = append(blocks, cover.ProfileBlock{
			StartLine: lineNumber,
			StartCol:  0,
			EndLine:   lineNumber,
			EndCol:    1,
			NumStmt:   1,
			Count:     0,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return []cover.ProfileBlock{{
			StartLine: 1,
			StartCol:  0,
			EndLine:   1,
			EndCol:    1,
			NumStmt:   1,
			Count:     0,
		}}, nil
	}
	return blocks, nil
}

func parseBaselineProfiles(path string) ([]*cover.Profile, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	var fileNames []string
	hasBaselineMarker := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "mode: ") {
			return nil, false, nil
		}
		switch {
		case strings.HasPrefix(line, "SF:"):
			hasBaselineMarker = true
			fileNames = append(fileNames, line[len("SF:"):])
		case strings.HasPrefix(line, "DA:"):
			// Full LCOV reports are handled by Bazel's default merger.
			return nil, false, nil
		case line == "end_of_record":
			continue
		default:
			// Ignore LH/LF/FN metadata from baseline files.
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	if !hasBaselineMarker {
		return nil, false, nil
	}

	profiles := make([]*cover.Profile, 0, len(fileNames))
	for _, fileName := range fileNames {
		blocks, err := zeroBlocksForBaselineFile(fileName)
		if err != nil {
			return nil, true, err
		}
		profiles = append(profiles, &cover.Profile{
			FileName: fileName,
			Mode:     "atomic",
			Blocks:   blocks,
		})
	}
	return profiles, true, nil
}

func writeProfiles(path, mode string, profilesByFile map[string]*cover.Profile) error {
	output, err := os.Create(path)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := fmt.Fprintf(output, "mode: %s\n", mode); err != nil {
		return err
	}

	fileNames := make([]string, 0, len(profilesByFile))
	for fileName := range profilesByFile {
		fileNames = append(fileNames, fileName)
	}
	sort.Strings(fileNames)

	for _, fileName := range fileNames {
		profile := profilesByFile[fileName]
		for _, block := range profile.Blocks {
			if _, err := fmt.Fprintf(
				output,
				"%s:%d.%d,%d.%d %d %d\n",
				profile.FileName,
				block.StartLine,
				block.StartCol,
				block.EndLine,
				block.EndCol,
				block.NumStmt,
				block.Count,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateReport(reportsFile, outputFile string) error {
	reportPaths, err := readReportPaths(reportsFile)
	if err != nil {
		return fmt.Errorf("read reports file: %w", err)
	}

	profilesByFile := make(map[string]*cover.Profile)
	mode := ""
	for _, reportPath := range reportPaths {
		profiles, isGoProfile, err := parseGoProfiles(reportPath)
		if err != nil {
			return fmt.Errorf("parse %s: %w", reportPath, err)
		}
		if isGoProfile {
			if err := mergeProfiles(profilesByFile, profiles, &mode); err != nil {
				return err
			}
			continue
		}

		baselineProfiles, isBaselineProfile, err := parseBaselineProfiles(reportPath)
		if err != nil {
			return fmt.Errorf("parse %s: %w", reportPath, err)
		}
		if isBaselineProfile {
			if err := mergeBaselineProfiles(profilesByFile, baselineProfiles, &mode); err != nil {
				return err
			}
		}
	}
	if mode == "" {
		mode = "atomic"
	}

	if err := writeProfiles(outputFile, mode, profilesByFile); err != nil {
		return fmt.Errorf("write merged profile: %w", err)
	}
	return nil
}

func main() {
	reportsFile := flag.String("reports_file", "", "file containing paths to coverage profiles")
	outputFile := flag.String("output_file", "", "merged coverage profile")
	flag.Parse()
	if *reportsFile == "" || *outputFile == "" {
		fmt.Fprintln(os.Stderr, "--reports_file and --output_file are required")
		os.Exit(1)
	}
	if err := generateReport(*reportsFile, *outputFile); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

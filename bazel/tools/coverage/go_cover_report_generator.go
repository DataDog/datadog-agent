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
		if !isGoProfile {
			continue
		}
		if err := mergeProfiles(profilesByFile, profiles, &mode); err != nil {
			return err
		}
	}
	if mode == "" {
		// Bazel runs the report generator separately for LCOV baseline files.
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

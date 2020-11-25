// +build ignore

package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
)

var (
	// CIncludePattern is the regex for #include headers of C files
	CIncludePattern = `^\s*#include\s+"(.*)"$`
	includeRegexp   *regexp.Regexp
)

func init() {
	includeRegexp = regexp.MustCompile(CIncludePattern)
}

// this program is intended to be called from go generate
// it will preprocess a .c file to replace all the `#include "file.h"` statements with the header files contents
// while making sure to only include a file once.
// you may optionally specify additional include directories to search
func main() {
	if len(os.Args[1:]) < 2 {
		panic("please use 'go run include_headers.go <c_file> <output_file> [include_dir]...'")
	}

	args := os.Args[1:]
	inputFile, err := filepath.Abs(args[0])
	if err != nil {
		log.Fatalf("unable to get absolute path to %s: %s", args[0], err)
	}
	outputFile, err := filepath.Abs(args[1])
	if err != nil {
		log.Fatalf("unable to get absolute path to %s: %s", args[1], err)
	}

	err = runProcessing(inputFile, outputFile, args[2:])
	if err != nil {
		log.Fatalf("error including headers: %s", err)
	}
	fmt.Printf("successfully included headers from %s => %s\n", inputFile, outputFile)
}

func runProcessing(inputFile, outputFile string, dirs []string) error {
	var includeDirs []string
	for _, d := range dirs {
		dir, err := filepath.Abs(d)
		if err != nil {
			return fmt.Errorf("unable to get absolute path to %s: %s", d, err)
		}
		includeDirs = append(includeDirs, dir)
	}

	if err := os.MkdirAll(filepath.Dir(outputFile), 0755); err != nil {
		return err
	}

	of, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error opening output file: %s", err)
	}
	defer of.Close()

	if err := of.Chmod(0644); err != nil {
		return fmt.Errorf("error setting mode on output file: %s", err)
	}

	includedFiles := make(map[string]struct{})
	if err := processIncludes(inputFile, of, includeDirs, includedFiles); err != nil {
		return fmt.Errorf("error processing includes: %s", err)
	}
	return nil
}

func processIncludes(path string, out io.Writer, includeDirs []string, includedFiles map[string]struct{}) error {
	if _, included := includedFiles[path]; included {
		return nil
	}
	includedFiles[path] = struct{}{}
	log.Printf("included %s\n", path)

	sourceReader, err := os.Open(path)
	if err != nil {
		return err
	}
	defer sourceReader.Close()

	scanner := bufio.NewScanner(sourceReader)
	for scanner.Scan() {
		match := includeRegexp.FindSubmatch(scanner.Bytes())
		if len(match) == 2 {
			headerName := string(match[1])
			headerPath, err := findInclude(path, headerName, includeDirs)
			if err != nil {
				return fmt.Errorf("error searching for header: %s", err)
			}
			if err := processIncludes(headerPath, out, includeDirs, includedFiles); err != nil {
				return err
			}
			continue
		}
		out.Write(scanner.Bytes())
		out.Write([]byte{'\n'})
	}
	return nil
}

func findInclude(srcPath string, headerName string, includeDirs []string) (string, error) {
	allDirs := append([]string{filepath.Dir(srcPath)}, includeDirs...)

	for _, dir := range allDirs {
		p := filepath.Join(dir, headerName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("file %s not found", headerName)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package usm

import (
	"bufio"
	"errors"
	"io/fs"
	"regexp"
	"strings"
)

const manifestFile = "META-INF/MANIFEST.MF"

var startClassRegexp = regexp.MustCompile(`^Start-Class: ([^ ]+)$`)

// getStartClassName parses JAR manifest files to get the Start-Class parameter.
func getStartClassName(fs fs.FS, filename string) (string, error) {
	manifest, err := fs.Open(filename)
	if err != nil {
		return "", err
	}
	defer manifest.Close()

	reader, err := SizeVerifiedReader(manifest)
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		matches := startClassRegexp.FindSubmatch(scanner.Bytes())
		if len(matches) == 2 {
			return string(matches[1]), nil
		}
	}

	return "", errors.New("could not find Start-Class")
}

// getClassPath gets the class path from the Java command line.
func getClassPath(args []string) []string {
	def := []string{"."}

	if len(args) < 2 {
		return def
	}

	// To make the classname detection logic below work on the first argument.
	prev := "--first=arg"
	cparg := ""
	for _, arg := range args[1:] {
		if prev == "-cp" || prev == "-classpath" || prev == "--class-path" {
			cparg = arg
			prev = arg
			continue
		}

		if strings.HasPrefix(arg, "--class-path=") {
			cparg = arg[len("--class-path="):]
			prev = arg
			continue
		}

		// Everything that follows is the jar/module and its arguments and
		// should be ignored.
		if arg == "-jar" || arg == "-m" || arg == "--module" {
			break
		}

		// Classname, everything that follows is an argument to the class and
		// should be ignored.
		if !strings.HasPrefix(arg, "-") {
			prevNeedsParam := strings.HasPrefix(prev, "--") && !strings.ContainsRune(prev, '=')
			if !prevNeedsParam {
				break
			}
		}

		prev = arg
	}
	if cparg == "" {
		return def
	}

	return strings.Split(cparg, ":")
}

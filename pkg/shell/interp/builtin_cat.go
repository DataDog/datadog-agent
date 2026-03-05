// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (r *Runner) builtinCat(args []string) error {
	showLineNumbers := false
	showEnds := false
	squeezeBlank := false
	var files []string
	endOfFlags := false

	for _, a := range args {
		if endOfFlags || (!strings.HasPrefix(a, "-") || a == "-") {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}

		switch a {
		case "-n", "--number":
			showLineNumbers = true
		case "-E", "--show-ends":
			showEnds = true
		case "-s", "--squeeze-blank":
			squeezeBlank = true
		default:
			return fmt.Errorf("flag %q is not allowed for command \"cat\"", a)
		}
	}

	catOutput := func(reader io.Reader) error {
		scanner := bufio.NewScanner(reader)
		lineNum := 0
		prevBlank := false
		for scanner.Scan() {
			line := scanner.Text()
			isBlank := line == ""

			if squeezeBlank && isBlank && prevBlank {
				continue
			}
			prevBlank = isBlank

			lineNum++
			if showLineNumbers {
				fmt.Fprintf(r.stdout, "%6d\t", lineNum)
			}
			if showEnds {
				fmt.Fprintf(r.stdout, "%s$\n", line)
			} else {
				fmt.Fprintln(r.stdout, line)
			}
		}
		return scanner.Err()
	}

	if len(files) == 0 {
		if err := catOutput(r.stdin); err != nil {
			r.exitCode = 1
			return nil
		}
		r.exitCode = 0
		return nil
	}

	hasError := false
	for _, f := range files {
		if f == "-" {
			if err := catOutput(r.stdin); err != nil {
				hasError = true
			}
			continue
		}
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.dir, path)
		}
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(r.stderr, "cat: %s: No such file or directory\n", f)
			hasError = true
			continue
		}
		if err := catOutput(file); err != nil {
			hasError = true
		}
		file.Close()
	}
	if hasError {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
	return nil
}

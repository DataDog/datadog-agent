// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

type rubyDetector struct {
	ctx DetectionContext
}

func newRubyDetector(ctx DetectionContext) detector {
	return &rubyDetector{ctx: ctx}
}

func (r rubyDetector) detect(args []string) (ServiceMetadata, bool) {
	// Check it is a Rails command.
	detectRails, ok := r.detectRails(args)
	if ok {
		return detectRails, true
	}

	// Fallback to the simple detector.
	simpleDetector := newSimpleDetector(r.ctx)
	return simpleDetector.detect(args)
}

func (r rubyDetector) detectRails(args []string) (ServiceMetadata, bool) {
	// Check if the command is a Rails command.
	// Rails commands are either `rails` or `bin/rails`.
	if len(args) < 1 || !strings.HasSuffix(strings.ToLower(args[0]), "rails") {
		return ServiceMetadata{}, false
	}

	// Detect checks if the current directory contains a Rails application by looking for a
	// `config/application.rb` file.
	// This file should contain a `module` declaration with the application name.
	cwd, _ := workingDirFromEnvs(r.ctx.envs)
	absFile := abs("config/application.rb", cwd)
	if _, err := os.Stat(absFile); err == nil {
		name, ok := r.findRailsApplicationName(absFile)
		if ok {
			return NewServiceMetadata(railsUnderscore(name)), true
		}
	}

	return ServiceMetadata{}, false
}

// findRailsApplicationName Scan the `config/application.rb` file to find the Rails application name.
func (r rubyDetector) findRailsApplicationName(filename string) (string, bool) {
	file, err := r.ctx.fs.Open(filename)
	if err != nil {
		return "", false
	}

	ok, err := canSafelyParse(file)
	if err != nil {
		return "", false
	}
	if !ok {
		r.ctx.logger.Debug("won't read file because it's too large", zap.String("filename", filename))
		return "", false
	}

	// Find the first `module Xyz` declaration in a file
	pattern := "module\\s+([A-Z][a-zA-Z0-9_]*)"
	re := regexp.MustCompile(pattern)

	// Scan the file line by line, instead of reading the whole file into memory
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		matches := re.FindStringSubmatch(scanner.Text())
		if matches != nil {
			return matches[1], true
		}
	}

	// No match found
	return "", false
}

// railsUnderscore Converts a PascalCasedWord to a snake_cased_word.
// It keeps uppercase acronyms together when converting (e.g. "HTTPServer" -> "http_server").
func railsUnderscore(pascalCasedWord string) string {
	var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

	snake := matchFirstCap.ReplaceAllString(pascalCasedWord, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

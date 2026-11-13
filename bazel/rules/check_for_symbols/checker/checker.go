// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main verifies a binary's symbol table contains/omits expected
// symbol patterns.
//
// It reads the symbol table of an unstripped Go binary (ELF, Mach-O or PE)
// directly, rather than shelling out to `nm`, which C++ toolchains do not
// reliably ship as an action input. The symbol names are checked against
// "must include" and "must not include" regex patterns, failing with a
// diagnostic on stderr if any expectation is violated.
package main

import (
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type patternList []string

func (p *patternList) String() string { return strings.Join(*p, ",") }

func (p *patternList) Set(v string) error {
	*p = append(*p, v)
	return nil
}

// readSymbols returns the names in the binary's symbol table, one per line.
func readSymbols(path string) (string, error) {
	var names []string
	if f, err := elf.Open(path); err == nil {
		defer f.Close()
		syms, err := f.Symbols()
		if err != nil && !errors.Is(err, elf.ErrNoSymbols) {
			return "", err
		}
		for _, s := range syms {
			names = append(names, s.Name)
		}
	} else if f, err := macho.Open(path); err == nil {
		defer f.Close()
		if f.Symtab != nil {
			for _, s := range f.Symtab.Syms {
				names = append(names, s.Name)
			}
		}
	} else if f, err := pe.Open(path); err == nil {
		defer f.Close()
		for _, s := range f.Symbols {
			names = append(names, s.Name)
		}
	} else {
		return "", fmt.Errorf("%s: not an ELF, Mach-O or PE binary", path)
	}
	return strings.Join(names, "\n"), nil
}

// goSymbol is present in every unstripped Go binary. Its absence means the
// symbol table was stripped and nothing can be verified.
var goSymbol = regexp.MustCompile(`(?m)runtime\.main$`)

func checkSymbols(symbols string, mustInclude, mustNotInclude []string) ([]string, error) {
	var failures []string
	for _, pattern := range mustInclude {
		re, err := regexp.Compile("(?m)" + pattern)
		if err != nil {
			return nil, err
		}
		if !re.MatchString(symbols) {
			failures = append(failures, fmt.Sprintf("expected a symbol matching %q, but none was found in the symbol table", pattern))
		}
	}
	for _, pattern := range mustNotInclude {
		re, err := regexp.Compile("(?m)" + pattern)
		if err != nil {
			return nil, err
		}
		if re.MatchString(symbols) {
			failures = append(failures, fmt.Sprintf("found a symbol matching %q, but it must not be present", pattern))
		}
	}
	return failures, nil
}

func run() error {
	var binary, output string
	var mustInclude, mustNotInclude patternList
	flag.StringVar(&binary, "binary", "", "Path to the binary to inspect.")
	flag.Var(&mustInclude, "must-include", "Regex pattern that must match at least one symbol. May be repeated.")
	flag.Var(&mustNotInclude, "must-not-include", "Regex pattern that must not match any symbol. May be repeated.")
	flag.StringVar(&output, "output", "", "Marker file written on success.")
	flag.Parse()
	if binary == "" || output == "" {
		return errors.New("--binary and --output are required")
	}

	symbols, err := readSymbols(binary)
	if err != nil {
		return err
	}
	if !goSymbol.MatchString(symbols) {
		return fmt.Errorf("%s: no runtime.main symbol; the binary is stripped or not a Go program", binary)
	}
	failures, err := checkSymbols(symbols, mustInclude, mustNotInclude)
	if err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("check_for_symbols failed for %s:\n%s", binary, strings.Join(failures, "\n"))
	}
	return os.WriteFile(output, []byte("PASS\n"), 0o644)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

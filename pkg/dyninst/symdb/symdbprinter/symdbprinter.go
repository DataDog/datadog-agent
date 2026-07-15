// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package symdbprinter renders an uploader.Scope (i.e. the wire form of a
// SymDB upload) as human-readable text. The rendering is suitable for golden
// snapshot tests and CLI inspection output: every byte the printer emits has
// a one-to-one correspondence with a field on the wire.
package symdbprinter

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
)

// SerializeScope writes a human-readable text representation of scope to w.
// Nested local scopes are recursively expanded to their full depth.
func SerializeScope(w io.Writer, scope uploader.Scope) (retErr error) {
	bw := bufio.NewWriter(w)
	p := &printer{w: bw}
	defer func() {
		if r := recover(); r != nil {
			if perr, ok := r.(printerError); ok {
				retErr = perr.err
				return
			}
			panic(r)
		}
	}()
	p.writeScope(scope, "")
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("symdbprinter: flush: %w", err)
	}
	return nil
}

// printer collects the recursive write helpers. Internal write errors are
// converted to panics with a printerError sentinel and recovered in
// SerializeScope, so the helpers can read like straight-line code.
type printer struct {
	w *bufio.Writer
}

type printerError struct{ err error }

func (p *printer) writeString(s string) {
	if _, err := p.w.WriteString(s); err != nil {
		panic(printerError{fmt.Errorf("symdbprinter: write: %w", err)})
	}
}

func (p *printer) writeByte(b byte) {
	if err := p.w.WriteByte(b); err != nil {
		panic(printerError{fmt.Errorf("symdbprinter: write: %w", err)})
	}
}

func (p *printer) printf(format string, args ...any) {
	if _, err := fmt.Fprintf(p.w, format, args...); err != nil {
		panic(printerError{fmt.Errorf("symdbprinter: write: %w", err)})
	}
}

// writeScope emits scope at the given indent. Children are indented one tab
// deeper.
func (p *printer) writeScope(scope uploader.Scope, indent string) {
	switch scope.ScopeType {
	case uploader.ScopeTypePackage:
		p.writeString(indent)
		p.writeString("Package: ")
		p.writeString(scope.Name)
		p.writeByte('\n')
	case uploader.ScopeTypeFunction, uploader.ScopeTypeMethod:
		p.writeFunctionHeader(scope, indent)
	case uploader.ScopeTypeStruct:
		p.writeString(indent)
		p.writeString("Type: ")
		p.writeString(scope.Name)
		p.writeByte('\n')
	case uploader.ScopeTypeLocal:
		p.printf("%sScope: %d-%d\n", indent, scope.StartLine, scope.EndLine)
	default:
		panic(printerError{fmt.Errorf("symdbprinter: unknown scope_type %q", scope.ScopeType)})
	}
	childIndent := indent + "\t"
	for _, sym := range scope.Symbols {
		p.writeSymbol(sym, childIndent)
	}
	for _, child := range scope.Scopes {
		p.writeScope(child, childIndent)
	}
}

// normalizeFilePath strips the version suffix from Go module cache paths so
// that snapshot files remain stable across dependency version bumps.
// e.g. "github.com/foo/bar@v1.2.3/pkg/file.go" → "github.com/foo/bar/pkg/file.go"
func normalizeFilePath(file string) string {
	at := strings.Index(file, "@v")
	if at < 0 {
		return file
	}
	slash := strings.Index(file[at:], "/")
	if slash < 0 {
		return file[:at]
	}
	return file[:at] + file[at+slash:]
}

func (p *printer) writeFunctionHeader(s uploader.Scope, indent string) {
	qualified := ""
	if s.LanguageSpecifics != nil {
		qualified = s.LanguageSpecifics.GoQualifiedName
	}
	p.printf("%sFunction: %s (%s) in %s [%d:%d] injectible: ",
		indent, s.Name, qualified, normalizeFilePath(s.SourceFile), s.StartLine, s.EndLine)
	p.writeRanges(s.InjectibleLines)
	p.writeByte('\n')
}

func (p *printer) writeSymbol(sym uploader.Symbol, indent string) {
	p.writeString(indent)
	p.writeString(symbolPrefix(sym.SymbolType))
	p.writeString(sym.Name)
	p.writeString(": ")
	p.writeString(sym.Type)
	if sym.SymbolType == uploader.SymbolTypeField {
		// Fields don't carry declared-line or availability metadata.
		p.writeByte('\n')
		return
	}
	declLine := 0
	if sym.Line != nil {
		declLine = *sym.Line
	}
	p.writeString(" (declared at line ")
	p.writeString(strconv.Itoa(declLine))
	p.writeString(", available: ")
	if sym.LanguageSpecifics != nil {
		p.writeRanges(sym.LanguageSpecifics.AvailableLineRanges)
	}
	p.writeString(")\n")
}

func (p *printer) writeRanges(ranges []uploader.LineRange) {
	for i, r := range ranges {
		if i > 0 {
			p.writeString(", ")
		}
		p.printf("[%d-%d]", r.Start, r.End)
	}
}

func symbolPrefix(sym uploader.SymbolType) string {
	switch sym {
	case uploader.SymbolTypeArg:
		return "Arg: "
	case uploader.SymbolTypeField:
		return "Field: "
	default:
		return "Var: "
	}
}

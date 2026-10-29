// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package sha256sum implements the sha256sum builtin command.
//
// sha256sum computes SHA-256 digests for regular files or standard input and
// verifies GNU-style or tagged SHA256 checksum manifests. File contents are
// streamed through a fixed-size buffer. Manifest line length, total manifest
// bytes, entry count, and operand count are bounded.
//
// Accepted flags:
//
//	-c, --check   read SHA-256 sums from files or standard input and verify them
//	    --quiet   do not print OK for successfully verified files
//	    --status  do not print per-file verification results
//	-h, --help    print usage to stdout and exit 0
package sha256sum

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/flagparser"
)

// Cmd is the sha256sum builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "sha256sum",
	Description: "compute and check SHA-256 message digests",
	MakeFlags:   registerFlags,
}

const (
	readBufferBytes     = 32 * 1024
	manifestBufferBytes = 4 * 1024

	// MaxManifestLineBytes bounds a single checksum line, including its path.
	MaxManifestLineBytes = 64 * 1024
	// MaxManifestBytes bounds all checksum manifests consumed by one command.
	MaxManifestBytes = 8 * 1024 * 1024
	// MaxManifestEntries bounds the number of target files verified per command.
	MaxManifestEntries = 1024
	// MaxOperands bounds direct file and checksum-manifest operands.
	MaxOperands = 1024
	// MaxCheckOutputBytes bounds dynamic per-entry verification output.
	MaxCheckOutputBytes = 1024 * 1024

	sha256HexBytes = sha256.Size * 2
)

var (
	errManifestTooLarge = fmt.Errorf("checksum manifests exceed %d-byte limit", MaxManifestBytes)
	errCheckOutputLimit = fmt.Errorf("verification output exceeds %d-byte limit", MaxCheckOutputBytes)
	errNoReadProgress   = errors.New("reader made no progress")
)

type readDeadlineSetter interface {
	SetReadDeadline(time.Time) error
}

type cancellableReader struct {
	ctx    context.Context
	reader io.Reader
}

type readResult struct {
	n   int
	err error
}

func (r *cancellableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	buf := make([]byte, len(p))
	done := make(chan readResult, 1)
	go func() {
		n, err := r.reader.Read(buf)
		done <- readResult{n: n, err: err}
	}()

	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	case result := <-done:
		copy(p, buf[:result.n])
		return result.n, result.err
	}
}

type options struct {
	quiet  bool
	status bool
}

type checksumEntry struct {
	digest string
	name   string
}

type checkTotals struct {
	manifestBytes int
	entries       int
	malformed     int
	mismatched    int
	unreadable    int
	outputBytes   int
}

type manifestBudgetReader struct {
	reader io.Reader
	totals *checkTotals
}

func (r *manifestBudgetReader) Read(p []byte) (int, error) {
	remaining := MaxManifestBytes - r.totals.manifestBytes
	if remaining < 0 {
		return 0, errManifestTooLarge
	}
	if len(p) > remaining+1 {
		p = p[:remaining+1]
	}

	n, err := r.reader.Read(p)
	if n > remaining {
		r.totals.manifestBytes += n
		return remaining, errManifestTooLarge
	}
	r.totals.manifestBytes += n
	return n, err
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := flagparser.RegisterNoArgBool(fs, "help", "h", "print usage and exit")
	check := flagparser.RegisterNoArgBool(fs, "check", "c", "read SHA-256 sums from files and check them")
	quiet := flagparser.RegisterNoArgBool(fs, "quiet", "", "do not print OK for each successfully verified file")
	status := flagparser.RegisterNoArgBool(fs, "status", "", "do not output per-file verification results")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		if !*check && (*quiet || *status) {
			flagName := "--quiet"
			if *status {
				flagName = "--status"
			}
			callCtx.Errf("sha256sum: the %s option is meaningful only when verifying checksums\n", flagName)
			callCtx.Errf("Try 'sha256sum --help' for more information.\n")
			return builtins.Result{Code: 1}
		}
		if len(args) > MaxOperands {
			callCtx.Errf("sha256sum: too many operands (maximum %d)\n", MaxOperands)
			return builtins.Result{Code: 1}
		}

		opts := options{quiet: *quiet, status: *status}
		if *check {
			return verifyManifests(ctx, callCtx, args, opts)
		}
		return generateDigests(ctx, callCtx, args)
	}
}

func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: sha256sum [OPTION]... [FILE]...\n")
	callCtx.Out("Print or check SHA-256 checksums.\n")
	callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")

	var saved []*builtins.Flag
	fs.VisitAll(func(flag *builtins.Flag) {
		if flag.NoOptDefVal == flagparser.NoArgSentinel {
			saved = append(saved, flag)
			flag.NoOptDefVal = ""
		}
	})
	defer func() {
		for _, flag := range saved {
			flag.NoOptDefVal = flagparser.NoArgSentinel
		}
	}()

	fs.SetOutput(callCtx.Stdout)
	fs.PrintDefaults()
}

func generateDigests(ctx context.Context, callCtx *builtins.CallContext, paths []string) builtins.Result {
	if len(paths) == 0 {
		paths = []string{"-"}
	}

	failed := false
	for _, path := range paths {
		if ctx.Err() != nil {
			return builtins.Result{Code: 1}
		}

		digest, err := digestPath(ctx, callCtx, path)
		if err != nil {
			name := path
			if path == "-" {
				name = "standard input"
			}
			callCtx.Errf("sha256sum: %s: %s\n", builtins.SafeOperand(name), portableError(callCtx, err))
			failed = true
			continue
		}

		name, escaped := escapeManifestName(path)
		prefix := ""
		if escaped {
			prefix = "\\"
		}
		callCtx.Out(prefix + digest + "  " + name + "\n")
	}

	if failed {
		return builtins.Result{Code: 1}
	}
	return builtins.Result{}
}

func verifyManifests(ctx context.Context, callCtx *builtins.CallContext, sources []string, opts options) builtins.Result {
	if len(sources) == 0 {
		sources = []string{"-"}
	}

	totals := checkTotals{}
	failed := false
	for _, source := range sources {
		if ctx.Err() != nil {
			return builtins.Result{Code: 1}
		}

		before := totals
		err := withReader(ctx, callCtx, source, func(r io.Reader) error {
			return verifyManifest(ctx, callCtx, r, opts, &totals, source == "-")
		})
		sourceTotals := checkTotals{
			entries:    totals.entries - before.entries,
			malformed:  totals.malformed - before.malformed,
			mismatched: totals.mismatched - before.mismatched,
			unreadable: totals.unreadable - before.unreadable,
		}
		if err != nil {
			callCtx.Errf("sha256sum: %s: %s\n", builtins.SafeOperand(sourceLabel(source)), portableError(callCtx, err))
			failed = true
		}
		if !opts.status && sourceTotals.entries > 0 {
			writeCheckWarnings(callCtx, sourceTotals)
		}
		if err != nil {
			continue
		}
		if sourceTotals.entries == 0 {
			callCtx.Errf("sha256sum: %s: no properly formatted checksum lines found\n", builtins.SafeOperand(sourceLabel(source)))
			failed = true
		}
	}

	if totals.mismatched > 0 || totals.unreadable > 0 {
		failed = true
	}
	if failed {
		return builtins.Result{Code: 1}
	}
	return builtins.Result{}
}

func verifyManifest(
	ctx context.Context,
	callCtx *builtins.CallContext,
	r io.Reader,
	opts options,
	totals *checkTotals,
	manifestIsStdin bool,
) error {
	reader := &manifestBudgetReader{reader: r, totals: totals}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, manifestBufferBytes), MaxManifestLineBytes+2)
	scanner.Split(splitManifestLines)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		if len(line) > MaxManifestLineBytes {
			return fmt.Errorf("checksum line exceeds %d-byte limit", MaxManifestLineBytes)
		}
		if line == "" {
			continue
		}

		entry, ok := parseChecksumLine(line)
		if !ok {
			totals.malformed++
			continue
		}
		if manifestIsStdin && entry.name == "-" {
			totals.malformed++
			continue
		}
		if totals.entries >= MaxManifestEntries {
			return fmt.Errorf("checksum manifests exceed %d-entry limit", MaxManifestEntries)
		}
		totals.entries++

		digest, err := digestPath(ctx, callCtx, entry.name)
		if err != nil {
			totals.unreadable++
			stdout := ""
			if !opts.status {
				stdout = formatCheckName(entry.name) + ": FAILED open or read\n"
			}
			stderr := "sha256sum: " + builtins.SafeOperand(entry.name) + ": " + portableError(callCtx, err) + "\n"
			if err := writeCheckOutput(callCtx, totals, stdout, stderr); err != nil {
				return err
			}
			continue
		}

		if digest != entry.digest {
			totals.mismatched++
			if !opts.status {
				if err := writeCheckOutput(callCtx, totals, formatCheckName(entry.name)+": FAILED\n", ""); err != nil {
					return err
				}
			}
			continue
		}
		if !opts.status && !opts.quiet {
			if err := writeCheckOutput(callCtx, totals, formatCheckName(entry.name)+": OK\n", ""); err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, errManifestTooLarge) {
			return errManifestTooLarge
		}
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("checksum line exceeds %d-byte limit", MaxManifestLineBytes)
		}
		return err
	}
	return nil
}

func splitManifestLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		switch b {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			if i+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func writeCheckOutput(callCtx *builtins.CallContext, totals *checkTotals, stdout, stderr string) error {
	remaining := MaxCheckOutputBytes - totals.outputBytes
	if len(stdout) > remaining || len(stderr) > remaining-len(stdout) {
		return errCheckOutputLimit
	}
	totals.outputBytes += len(stdout) + len(stderr)
	if stdout != "" {
		callCtx.Out(stdout)
	}
	if stderr != "" {
		callCtx.Errf("%s", stderr)
	}
	return nil
}

func writeCheckWarnings(callCtx *builtins.CallContext, totals checkTotals) {
	if totals.malformed > 0 {
		callCtx.Errf("sha256sum: WARNING: %d %s improperly formatted\n",
			totals.malformed, plural(totals.malformed, "line is", "lines are"))
	}
	if totals.unreadable > 0 {
		callCtx.Errf("sha256sum: WARNING: %d listed %s not be read\n",
			totals.unreadable, plural(totals.unreadable, "file could", "files could"))
	}
	if totals.mismatched > 0 {
		callCtx.Errf("sha256sum: WARNING: %d computed %s NOT match\n",
			totals.mismatched, plural(totals.mismatched, "checksum did", "checksums did"))
	}
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

func digestPath(ctx context.Context, callCtx *builtins.CallContext, path string) (string, error) {
	var digest string
	err := withReader(ctx, callCtx, path, func(r io.Reader) error {
		var err error
		digest, err = digestReader(ctx, r)
		return err
	})
	return digest, err
}

func withReader(
	ctx context.Context,
	callCtx *builtins.CallContext,
	path string,
	fn func(io.Reader) error,
) error {
	if path == "" {
		return errors.New("invalid zero-length file name")
	}
	if path == "-" {
		if callCtx.Stdin == nil {
			return fn(strings.NewReader(""))
		}
		return withCancellableReader(ctx, callCtx.Stdin, fn)
	}
	if callCtx.OpenRegularFile == nil {
		return errors.New("regular-file capability not available")
	}

	r, err := callCtx.OpenRegularFile(ctx, path)
	if err != nil {
		return err
	}
	defer r.Close()
	return fn(r)
}

func withCancellableReader(ctx context.Context, r io.Reader, fn func(io.Reader) error) error {
	if ctx.Done() == nil {
		return fn(r)
	}
	deadlineReader, ok := r.(readDeadlineSetter)
	if !ok || deadlineReader.SetReadDeadline(time.Time{}) != nil {
		return fn(&cancellableReader{ctx: ctx, reader: r})
	}
	deadlineInstalled := false
	if deadline, ok := ctx.Deadline(); ok {
		deadlineInstalled = deadlineReader.SetReadDeadline(deadline) == nil
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = deadlineReader.SetReadDeadline(time.Unix(1, 0))
		case <-stop:
		}
	}()
	defer func() {
		close(stop)
		<-done
		_ = deadlineReader.SetReadDeadline(time.Time{})
	}()
	err := fn(r)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if deadlineInstalled {
			<-ctx.Done()
			return ctx.Err()
		}
	}
	return err
}

func digestReader(ctx context.Context, r io.Reader) (string, error) {
	h := sha256.New()
	buf := make([]byte, readBufferBytes)
	emptyReads := 0
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		n, err := r.Read(buf)
		if n > 0 {
			emptyReads = 0
			_, _ = h.Write(buf[:n])
		} else if err == nil {
			emptyReads++
			if emptyReads >= 100 {
				return "", errNoReadProgress
			}
		}
		if errors.Is(err, io.EOF) {
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if err != nil {
			return "", err
		}
	}
}

func parseChecksumLine(line string) (checksumEntry, bool) {
	for len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
		line = line[1:]
	}
	escaped := strings.HasPrefix(line, "\\")
	if escaped {
		line = line[1:]
	}

	entry, ok := parseUntaggedLine(line)
	if !ok {
		entry, ok = parseTaggedLine(line)
	}
	if !ok {
		return checksumEntry{}, false
	}
	if escaped {
		name, valid := unescapeManifestName(entry.name)
		if !valid {
			return checksumEntry{}, false
		}
		entry.name = name
	}
	entry.digest = strings.ToLower(entry.digest)
	return entry, true
}

func parseUntaggedLine(line string) (checksumEntry, bool) {
	if len(line) <= sha256HexBytes || !validHexDigest(line[:sha256HexBytes]) ||
		(line[sha256HexBytes] != ' ' && line[sha256HexBytes] != '\t') {
		return checksumEntry{}, false
	}

	nameStart := sha256HexBytes + 1
	if nameStart < len(line) && (line[nameStart] == ' ' || line[nameStart] == '*') {
		nameStart++
	}
	if nameStart >= len(line) {
		return checksumEntry{}, false
	}
	return checksumEntry{digest: line[:sha256HexBytes], name: line[nameStart:]}, true
}

func parseTaggedLine(line string) (checksumEntry, bool) {
	for _, format := range []struct {
		prefix    string
		separator string
	}{
		{prefix: "SHA256 (", separator: ") = "},
		{prefix: "SHA256(", separator: ")= "},
	} {
		if !strings.HasPrefix(line, format.prefix) || len(line) < len(format.prefix)+len(format.separator)+sha256HexBytes+1 {
			continue
		}
		digestStart := len(line) - sha256HexBytes
		separatorStart := digestStart - len(format.separator)
		if separatorStart < len(format.prefix) || line[separatorStart:digestStart] != format.separator {
			continue
		}
		digest := line[digestStart:]
		if !validHexDigest(digest) {
			continue
		}
		return checksumEntry{digest: digest, name: line[len(format.prefix):separatorStart]}, true
	}
	return checksumEntry{}, false
}

func validHexDigest(s string) bool {
	if len(s) != sha256HexBytes {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9') && !('a' <= c && c <= 'f') && !('A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

func escapeManifestName(name string) (string, bool) {
	var b strings.Builder
	escaped := false
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '\\':
			b.WriteString("\\\\")
			escaped = true
		case '\n':
			b.WriteString("\\n")
			escaped = true
		case '\r':
			b.WriteString("\\r")
			escaped = true
		default:
			b.WriteByte(name[i])
		}
	}
	if !escaped {
		return name, false
	}
	return b.String(), true
}

func unescapeManifestName(name string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] != '\\' {
			b.WriteByte(name[i])
			continue
		}
		i++
		if i >= len(name) {
			return "", false
		}
		switch name[i] {
		case '\\':
			b.WriteByte('\\')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			return "", false
		}
	}
	return b.String(), true
}

func formatCheckName(name string) string {
	if utf8.ValidString(name) {
		safe := builtins.SafeOperand(name)
		if safe == name {
			return name
		}
		return "\\" + safe
	}

	const hexDigits = "0123456789abcdef"
	var b strings.Builder
	start := 0
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		if r != utf8.RuneError || size != 1 {
			i += size
			continue
		}

		safe := builtins.SafeOperand(name[start:i])
		b.WriteString(safe)
		b.WriteString("\\x")
		b.WriteByte(hexDigits[name[i]>>4])
		b.WriteByte(hexDigits[name[i]&0x0f])
		i++
		start = i
	}
	safe := builtins.SafeOperand(name[start:])
	b.WriteString(safe)
	return "\\" + b.String()
}

func sourceLabel(source string) string {
	if source == "-" {
		return "'standard input'"
	}
	return source
}

func portableError(callCtx *builtins.CallContext, err error) string {
	if callCtx.PortableErr != nil {
		return callCtx.PortableErr(err)
	}
	return err.Error()
}

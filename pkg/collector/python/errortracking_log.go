// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package python

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// This file turns Python tracebacks into the redacted stacks submitted to
// error tracking. It is pure Go (no cgo, no interpreter) so it can be tested
// anywhere.
//
// Privacy rule: only frames of Datadog-shipped integration packages are
// shipped, with package-relative paths. Frames from custom checks and from
// any other Python code (standard library, third-party packages) are
// customer code or reveal the installation layout, so they are replaced by a
// placeholder frame. Exception messages, source lines, local values and
// absolute paths are never read into the result.

const (
	// shippedPackageRoot is the namespace package that holds Datadog-shipped
	// integrations once installed under site-packages.
	shippedPackageRoot = "datadog_checks/"

	// loggingBridgeFile is the integrations base module that forwards Python
	// log records to the Agent; its frames are plumbing, not the root cause.
	loggingBridgeFile = "datadog_checks/base/log.py"

	// Python log levels, as sent by the integrations logging bridge.
	pythonLevelError    = 40
	pythonLevelCritical = 50

	// stackContextKey is the log context key carrying the redacted stack. The
	// log formatters show the key with the integration name as its value.
	stackContextKey = "integration"

	customCheckFrame = "<custom_check>"
	externalFrame    = "<external>"
	redactedLocation = "<redacted>:0 +0x0"

	// customSource and baseSource are the Source values for stacks without a
	// non-base shipped integration frame.
	customSource = "custom"
	baseSource   = "base"
)

// shippedIntegrationsText lists the Datadog-shipped integration packages. See
// the header of the file for what it is and how it is refreshed.
//
//go:embed shipped_integrations.txt
var shippedIntegrationsText string

var (
	// shippedIntegrations holds the names of Datadog-shipped integrations.
	shippedIntegrations = parseNameList(shippedIntegrationsText)

	// basePackages are the subpackages of the integrations base library, which
	// is shipped by Datadog with every Agent.
	basePackages = map[string]struct{}{"base": {}, "checks": {}, "utils": {}}

	// tracebackFrameRe matches a frame line of a formatted Python traceback.
	// Python indents frames by exactly two spaces.
	tracebackFrameRe = regexp.MustCompile(`^  File "(.*)", line (\d+)(?:, in (.*))?$`)

	// shippedPathRe restricts shipped relative paths to a conservative
	// character set so nothing unexpected can reach the wire.
	shippedPathRe = regexp.MustCompile(`^[A-Za-z0-9_]+(/[A-Za-z0-9_]+)*\.py$`)

	// functionNameRe accepts Python function names including synthetic ones
	// such as <module> and <listcomp>.
	functionNameRe = regexp.MustCompile(`^[A-Za-z_<][A-Za-z0-9_<>.]*$`)

	// exceptionTypeRe accepts a dotted exception class name.
	exceptionTypeRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
)

const (
	tracebackHeader = "Traceback (most recent call last):"

	// Lines Python prints between the tracebacks of chained exceptions.
	chainedCause   = "The above exception was the direct cause of the following exception:"
	chainedContext = "During handling of the above exception, another exception occurred:"
)

// parseNameList returns the set of names in text, one per line, ignoring
// blank lines and lines starting with #.
func parseNameList(text string) map[string]struct{} {
	names := make(map[string]struct{})
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			names[l] = struct{}{}
		}
	}
	return names
}

// tracebackFrame is one parsed frame, in the order Python prints them.
type tracebackFrame struct {
	path     string
	line     int
	function string
}

// parseTraceback extracts frames and the exception type from text formatted
// like traceback.format_exc output, or from a bare list of frames. Frames are
// returned outermost first. The exception message after the colon is
// discarded.
//
// The text can hold attacker-influenced lines (the exception message is
// printed verbatim, possibly over several lines), so the parser only follows
// the structure Python prints: the frames of a traceback end at the first
// unindented line, which is the exception line, and a further traceback is
// only accepted after one of the lines Python prints between chained
// exceptions. For chained exceptions the last traceback is used. Tracebacks
// of exception groups, whose lines are prefixed by "| ", do not match and
// yield no frames.
func parseTraceback(text string) (frames []tracebackFrame, excType string) {
	done, chained := false, false
	for _, l := range strings.Split(text, "\n") {
		if l == tracebackHeader {
			if done && !chained {
				continue
			}
			frames, excType, done, chained = nil, "", false, false
			continue
		}
		if done {
			switch {
			case l == chainedCause || l == chainedContext:
				chained = true
			case l != "":
				chained = false
			}
			continue
		}
		if m := tracebackFrameRe.FindStringSubmatch(l); m != nil {
			n, err := strconv.Atoi(m[2])
			if err != nil {
				n = 0
			}
			frames = append(frames, tracebackFrame{path: m[1], line: n, function: m[3]})
			continue
		}
		if len(frames) > 0 && l != "" && l[0] != ' ' && l[0] != '\t' {
			name, _, _ := strings.Cut(l, ":")
			if name = strings.TrimSpace(name); exceptionTypeRe.MatchString(name) {
				excType = name
			}
			done = true
		}
	}
	return frames, excType
}

type frameKind int

const (
	kindExternal frameKind = iota
	kindCustom
	kindShipped
)

// classifyFramePath decides how a frame is treated from its file path alone.
// For shipped frames it returns the package-relative path (for example
// "datadog_checks/redisdb/redisdb.py").
func classifyFramePath(path string) (rel string, kind frameKind) {
	path = strings.ReplaceAll(path, `\`, "/")
	if i := strings.LastIndex(path, "/site-packages/"); i >= 0 {
		rel = path[i+len("/site-packages/"):]
		if strings.HasPrefix(rel, shippedPackageRoot) && shippedPathRe.MatchString(rel) && isShippedPackage(rel) {
			return rel, kindShipped
		}
	}
	if strings.HasPrefix(path, "checks.d/") || strings.Contains(path, "/checks.d/") {
		return "", kindCustom
	}
	return "", kindExternal
}

// isShippedPackage reports whether rel, a path under the datadog_checks
// namespace, belongs to a package Datadog ships: the integrations base library
// or an integration on the shipped list. Anything else in the namespace may be
// a partner or customer package.
func isShippedPackage(rel string) bool {
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return false
	}
	_, base := basePackages[parts[1]]
	_, shipped := shippedIntegrations[parts[1]]
	return base || shipped
}

// redactExceptionType keeps an exception type name only when it cannot reveal
// customer code: builtin exceptions (no module) and exceptions defined by
// shipped packages. For any other module only the class name is kept, behind
// an external marker, since the module can be a customer package name.
func redactExceptionType(t string) string {
	module, class, found := cutLast(t, ".")
	if !found {
		return t
	}
	if strings.HasPrefix(module, "datadog_checks.") {
		pkg, _, _ := strings.Cut(strings.TrimPrefix(module, "datadog_checks."), ".")
		_, base := basePackages[pkg]
		_, shipped := shippedIntegrations[pkg]
		if base || shipped {
			return t
		}
	}
	return externalFrame + "." + class
}

// cutLast splits s around the last occurrence of sep.
func cutLast(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// buildExternalStack redacts frames (outermost first, as parsed) and formats
// them innermost first in the wire shape expected by error tracking:
// "module.function\n\tpath/file.py:line +0x0". It returns false when no
// frame with information is left to ship.
//
// With trimLoggingPlumbing set, the leading frames that belong to the logging
// machinery (standard library logging and the integrations logging bridge)
// are dropped so the first frame is the code that logged. Consecutive
// redacted frames collapse into one placeholder, and the result is capped at
// errortracking.MaxStackFrames frames.
func buildExternalStack(frames []tracebackFrame, excType string, trimLoggingPlumbing bool) (errortracking.ExternalStack, bool) {
	var out []string
	var lastPlaceholder string
	source := ""
	sawCustom, sawShipped := false, false
	leading := trimLoggingPlumbing

	for i := len(frames) - 1; i >= 0 && len(out) < errortracking.MaxStackFrames; i-- {
		f := frames[i]
		rel, kind := classifyFramePath(f.path)
		if leading {
			if kind == kindExternal || rel == loggingBridgeFile {
				continue
			}
			leading = false
		}
		switch kind {
		case kindShipped:
			lastPlaceholder = ""
			sawShipped = true
			out = append(out, formatShippedFrame(rel, f))
			if name := integrationName(rel); source == "" && name != baseSource {
				source = name
			}
		default:
			placeholder := externalFrame
			if kind == kindCustom {
				placeholder = customCheckFrame
				sawCustom = true
			}
			if placeholder == lastPlaceholder {
				continue
			}
			lastPlaceholder = placeholder
			out = append(out, placeholder+"\n\t"+redactedLocation)
		}
	}
	// A stack made only of placeholders carries no information and would
	// merge unrelated errors into one issue.
	if !sawShipped && !sawCustom {
		return errortracking.ExternalStack{}, false
	}
	switch {
	case source != "":
	case sawCustom:
		source = customSource
	default:
		source = baseSource
	}
	return errortracking.ExternalStack{
		Frames:    strings.Join(out, "\n"),
		ErrorKind: excType,
		Source:    source,
	}, true
}

// formatShippedFrame formats a frame of a shipped package. rel has already
// passed shippedPathRe, so it is safe to embed.
func formatShippedFrame(rel string, f tracebackFrame) string {
	module := strings.ReplaceAll(strings.TrimSuffix(rel, ".py"), "/", ".")
	module = strings.TrimSuffix(module, ".__init__")
	fn := f.function
	if !functionNameRe.MatchString(fn) {
		fn = "unknown"
	}
	return fmt.Sprintf("%s.%s\n\t%s:%d +0x0", module, fn, rel, max(f.line, 0))
}

// integrationName returns the integration a shipped path belongs to, or the
// base package name for the integrations base library.
func integrationName(rel string) string {
	parts := strings.Split(rel, "/")
	if _, base := basePackages[parts[1]]; base {
		return baseSource
	}
	return parts[1]
}

// stackFromCapture builds the stack for a log record from the frames text and
// exception type captured by the interpreter bridge at log time.
func stackFromCapture(stack, excType string) (errortracking.ExternalStack, bool) {
	frames, _ := parseTraceback(stack)
	if !exceptionTypeRe.MatchString(excType) {
		excType = ""
	}
	return buildExternalStack(frames, redactExceptionType(excType), true)
}

// logMessageWithStack logs an error or critical message emitted by Python
// code through the agent logger, attaching the redacted Python stack so error
// tracking reports the Python call site instead of this bridge. It returns
// false, without logging, when the level carries no stack or nothing
// shippable can be derived from it; the caller then logs the plain message.
func logMessageWithStack(level int, message, stack, excType string) bool {
	if level != pythonLevelError && level != pythonLevelCritical {
		return false
	}
	ext, ok := stackFromCapture(stack, excType)
	if !ok {
		return false
	}
	logc := log.Errorc
	if level == pythonLevelCritical {
		logc = log.Criticalc
	}
	_ = logc(message, stackContextKey, ext)
	return true
}

// checkError is the error returned by a Python check run that raised. Its
// text is the original run result, unchanged, so status pages and expvars
// keep showing what they showed before; the redacted stack travels alongside
// for error tracking.
type checkError struct {
	msg   string
	stack errortracking.ExternalStack
}

var _ errortracking.StackProvider = (*checkError)(nil)

func (e *checkError) Error() string {
	return e.msg
}

// ExternalStack returns the redacted stack of the raised exception.
func (e *checkError) ExternalStack() errortracking.ExternalStack {
	return e.stack
}

// newCheckError converts the non-empty result of a check run into an error.
// The result is the JSON report produced by the integrations base class; when
// it cannot be parsed or has no usable traceback a plain error is returned.
func newCheckError(result string) error {
	var report []struct {
		Traceback string `json:"traceback"`
	}
	if err := json.Unmarshal([]byte(result), &report); err == nil && len(report) > 0 {
		frames, excType := parseTraceback(report[0].Traceback)
		if stack, ok := buildExternalStack(frames, redactExceptionType(excType), false); ok {
			return &checkError{msg: result, stack: stack}
		}
	}
	return errors.New(result)
}

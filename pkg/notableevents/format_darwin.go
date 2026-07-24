// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	notableeventtypes "github.com/DataDog/datadog-agent/pkg/notableevents/types"
)

const (
	macosCrashBugType = "309"
	// User-derived strings are capped well below the 16 KiB event limit. With
	// the fixed allowlists below, even a maximally populated event remains
	// bounded while preserving useful diagnostic values.
	maxDarwinEventStringBytes = notableeventtypes.MaxEventStringBytes
	maxDarwinProcessNameBytes = 128
	unknownDarwinProcessName  = "Unknown application"
)

var macosMetadataKeys = []string{
	"app_name",
	"app_version",
	"build_version",
	"bug_type",
	"bundleID",
	"incident_id",
	"is_first_party",
	"name",
	"os_version",
	"platform",
	"share_with_app_devs",
	"slice_uuid",
	"timestamp",
}

var macosReportScalarKeys = []string{
	"bug_type",
	"captureTime",
	"coalitionID",
	"coalitionName",
	"cpuType",
	"deployVersion",
	"incident",
	"modelCode",
	"parentPid",
	"parentProc",
	"pid",
	"procLaunch",
	"procName",
	"procRole",
	"procStartAbsTime",
	"responsiblePid",
	"responsibleProc",
	"uptime",
	"version",
}

var macosNestedReportKeys = map[string][]string{
	"bundleInfo": {
		"CFBundleIdentifier",
		"CFBundleShortVersionString",
		"CFBundleVersion",
	},
	"exception": {
		"signal",
		"subtype",
		"type",
	},
	"termination": {
		"code",
		"namespace",
	},
}

// Event aliases the pure-Go wire contract shared with the core Agent.
type Event = notableeventtypes.Event

type macOSCrashReport struct {
	metadata map[string]interface{}
	report   map[string]interface{}
}

// diagnosticReportJSONError retains parse position context so truncated JSON
// can be retried without treating permanently malformed reports as incomplete.
type diagnosticReportJSONError struct {
	err          error
	contentBytes int
}

func (e *diagnosticReportJSONError) Error() string {
	return e.err.Error()
}

func (e *diagnosticReportJSONError) Unwrap() error {
	return e.err
}

func newDiagnosticReportJSONError(err error, data []byte) error {
	if err == nil {
		return nil
	}
	return &diagnosticReportJSONError{
		err:          err,
		contentBytes: len(bytes.TrimSpace(data)),
	}
}

// isIncompleteDiagnosticReportJSONError identifies only parse failures that
// indicate input ended while a JSON value was still unfinished.
func isIncompleteDiagnosticReportJSONError(err error) bool {
	var parseErr *diagnosticReportJSONError
	if !errors.As(err, &parseErr) {
		return false
	}
	if errors.Is(parseErr.err, io.EOF) || errors.Is(parseErr.err, io.ErrUnexpectedEOF) {
		return true
	}
	var syntaxErr *json.SyntaxError
	return errors.As(parseErr.err, &syntaxErr) &&
		syntaxErr.Error() == "unexpected end of JSON input" &&
		syntaxErr.Offset >= int64(parseErr.contentBytes)
}

// parseMacOSCrashMetadata parses the metadata object from the first line of a macOS crash report.
func parseMacOSCrashMetadata(data []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	metadata, err := decodeJSONMap(decoder)
	if err != nil {
		return nil, fmt.Errorf("failed to decode metadata object: %w", newDiagnosticReportJSONError(err, data))
	}
	return metadata, nil
}

// parseMacOSCrashReportBody parses the crash report body object from a macOS crash report.
func parseMacOSCrashReportBody(data []byte) (map[string]interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	report, err := decodeJSONMap(decoder)
	if err != nil {
		return nil, fmt.Errorf("failed to decode report object: %w", newDiagnosticReportJSONError(err, data))
	}
	return report, nil
}

// decodeJSONMap decodes one JSON object while preserving numeric precision.
func decodeJSONMap(decoder *json.Decoder) (map[string]interface{}, error) {
	var value map[string]interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return normalizeJSONMap(value), nil
}

// normalizeJSONMap converts decoded numeric values into stable Go scalar types.
func normalizeJSONMap(source map[string]interface{}) map[string]interface{} {
	return normalizeJSONNumbers(source).(map[string]interface{})
}

// normalizeJSONNumbers recursively normalizes JSON numbers in maps and slices.
func normalizeJSONNumbers(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, child := range typed {
			out[key] = normalizeJSONNumbers(child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index, child := range typed {
			out[index] = normalizeJSONNumbers(child)
		}
		return out
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		if number, err := typed.Float64(); err == nil {
			return number
		}
		return typed.String()
	default:
		return value
	}
}

// appName selects the best non-sensitive application name available in the report.
func (r *macOSCrashReport) appName() string {
	for _, candidate := range []string{
		getString(r.report, "procName"),
		getString(r.metadata, "name"),
		getString(r.metadata, "app_name"),
		appNameFromPath(getString(r.report, "procPath")),
	} {
		if sanitized, ok := sanitizeProcessName(candidate); ok {
			return sanitized
		}
	}
	return unknownDarwinProcessName
}

// incidentID returns the report's incident identifier when present.
func (r *macOSCrashReport) incidentID() string {
	if id := getString(r.metadata, "incident_id"); id != "" {
		return id
	}
	return getString(r.report, "incident")
}

// captureTime extracts the first valid report timestamp or uses the current time.
func (r *macOSCrashReport) captureTime() time.Time {
	for _, candidate := range []string{
		getString(r.report, "captureTime"),
		getString(r.metadata, "timestamp"),
	} {
		if timestamp, ok := parseMacOSTimestamp(candidate); ok {
			return timestamp
		}
	}
	return time.Now().UTC()
}

// event converts a parsed crash report into a sanitized notable event.
func (r *macOSCrashReport) event(identity, scope string) Event {
	appName := r.appName()
	title := boundedString("Application crash: "+appName, maxDarwinEventStringBytes)

	return Event{
		ID:        eventID(identity),
		Timestamp: r.captureTime(),
		EventType: "Application crash",
		Title:     title,
		Message:   "An application crashed unexpectedly",
		Custom: map[string]interface{}{
			"macos_diagnostic_report": r.customPayload(scope),
		},
	}
}

// customPayload builds the allowlisted macOS diagnostic-report payload.
func (r *macOSCrashReport) customPayload(scope string) map[string]interface{} {
	payload := map[string]interface{}{
		"metadata": copySelectedScalarFields(r.metadata, macosMetadataKeys),
		"report":   filteredReportFields(r.report),
		"scope":    sanitizeScope(scope),
	}

	payload["app_name"] = r.appName()
	if bundleID := sanitizePayloadString(firstNonEmpty(getString(r.metadata, "bundleID"), getString(r.report, "bundleID"))); bundleID != "" {
		payload["bundle_id"] = bundleID
	}
	if incidentID := sanitizePayloadString(r.incidentID()); incidentID != "" {
		payload["incident_id"] = incidentID
	}
	if procPath := getString(r.report, "procPath"); procPath != "" {
		if hint := appLocationHint(procPath); hint != "" {
			payload["app_location_hint"] = hint
		}
		if pathAppName, ok := sanitizeProcessName(appNameFromPath(procPath)); ok {
			payload["executable_basename"] = pathAppName
		}
	}

	return payload
}

// filteredReportFields copies only approved scalar and nested report fields.
func filteredReportFields(source map[string]interface{}) map[string]interface{} {
	out := copySelectedScalarFields(source, macosReportScalarKeys)
	for key, allowedKeys := range macosNestedReportKeys {
		value, ok := source[key].(map[string]interface{})
		if !ok {
			continue
		}
		filtered := copySelectedScalarFields(value, allowedKeys)
		if len(filtered) != 0 {
			out[key] = filtered
		}
	}
	return out
}

// copySelectedScalarFields copies allowlisted scalar values from a JSON object.
func copySelectedScalarFields(source map[string]interface{}, keys []string) map[string]interface{} {
	out := make(map[string]interface{})
	for _, key := range keys {
		value, ok := source[key]
		if !ok || !isJSONScalar(value) {
			continue
		}
		if text, isString := value.(string); isString {
			if sanitized := sanitizePayloadString(text); sanitized != "" {
				out[key] = sanitized
			}
			continue
		}
		out[key] = value
	}
	return out
}

// isJSONScalar reports whether a value is safe to include as a scalar payload field.
func isJSONScalar(value interface{}) bool {
	switch value.(type) {
	case nil, bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

// getString converts a supported scalar report field into its string representation.
func getString(source map[string]interface{}, key string) string {
	value, ok := source[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

// firstNonEmpty returns the first populated candidate string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// parseMacOSTimestamp parses timestamp layouts emitted by macOS diagnostic reports.
func parseMacOSTimestamp(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}

	layouts := []string{
		"2006-01-02 15:04:05.0000 -0700",
		"2006-01-02 15:04:05.000 -0700",
		"2006-01-02 15:04:05.00 -0700",
		"2006-01-02 15:04:05 -0700",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if timestamp, err := time.Parse(layout, value); err == nil {
			return timestamp, true
		}
	}
	return time.Time{}, false
}

// appNameFromPath derives an application label without exposing parent path components.
func appNameFromPath(procPath string) string {
	clean := filepath.Clean(procPath)
	var relative string
	switch {
	case strings.HasPrefix(clean, "/System/Applications/"):
		relative = strings.TrimPrefix(clean, "/System/Applications/")
	case strings.HasPrefix(clean, "/Applications/"):
		relative = strings.TrimPrefix(clean, "/Applications/")
	case strings.HasPrefix(clean, "/Users/"):
		parts := strings.Split(strings.TrimPrefix(clean, "/Users/"), "/")
		if len(parts) < 3 || parts[0] == "" || parts[1] != "Applications" {
			return ""
		}
		relative = strings.Join(parts[2:], "/")
	default:
		return ""
	}

	parts := strings.Split(relative, "/")
	for _, part := range parts {
		if strings.EqualFold(filepath.Ext(part), ".app") {
			return strings.TrimSuffix(part, filepath.Ext(part))
		}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return filepath.Base(clean)
}

// sanitizeProcessName accepts labels only, never paths or terminal-control
// content. Invalid report values fall through to another source or the fixed
// generic application name.
func sanitizeProcessName(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || strings.HasPrefix(value, "~") ||
		strings.ContainsAny(value, `/\`) {
		return "", false
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", false
		}
	}
	return boundedString(value, maxDarwinProcessNameBytes), true
}

// sanitizePayloadString bounds allowlisted report values and rejects path-like
// content so executable paths and usernames embedded in paths cannot leak.
func sanitizePayloadString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, `/\`) {
		return ""
	}
	var sanitized strings.Builder
	for _, char := range value {
		if unicode.IsControl(char) {
			sanitized.WriteByte(' ')
			continue
		}
		// encoding/json expands these runes substantially. Replacing them
		// keeps the serialized event limit coherent with the string limit.
		if char == '<' || char == '>' || char == '&' || char == '\u2028' || char == '\u2029' {
			sanitized.WriteByte('_')
			continue
		}
		sanitized.WriteRune(char)
	}
	return boundedString(strings.TrimSpace(sanitized.String()), maxDarwinEventStringBytes)
}

// boundedString truncates at a UTF-8 boundary.
func boundedString(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

func sanitizeScope(scope string) string {
	switch scope {
	case "system", "user":
		return scope
	default:
		return "unknown"
	}
}

// appLocationHint returns only coarse categories for recognized roots. It
// never returns any component of the executable path.
func appLocationHint(procPath string) string {
	clean := filepath.Clean(procPath)
	switch {
	case strings.HasPrefix(clean, "/System/Applications/"):
		return "system_application"
	case strings.HasPrefix(clean, "/Applications/"):
		return "application"
	case strings.HasPrefix(clean, "/private/tmp/"),
		strings.HasPrefix(clean, "/private/var/folders/"),
		strings.HasPrefix(clean, "/tmp/"),
		strings.HasPrefix(clean, "/var/tmp/"):
		return "temporary"
	case strings.HasPrefix(clean, "/Volumes/"):
		return "external"
	case strings.HasPrefix(clean, "/Users/"):
		if clean != procPath {
			return ""
		}
		parts := strings.Split(strings.TrimPrefix(clean, "/Users/"), "/")
		if len(parts) < 3 || parts[0] == "" || parts[1] != "Applications" || parts[2] == "" {
			return ""
		}
		return "user_application"
	default:
		return ""
	}
}

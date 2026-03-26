// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package experimental implements undocumented experimental CLI subcommands.
package experimental

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var apiKeyRegex = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// validSiteRe matches any known Datadog site domain using the same pattern as
// pkg/config/utils/endpoints.go (ddSitePattern + ddDomainPattern).
// It anchors both ends so that e.g. "notdatadoghq.com" is rejected.
var validSiteRe = regexp.MustCompile(`^(?:[a-z]{2,}\d{1,2}\.)?(?:datad(?:oghq|0g)\.(?:com|eu)|ddog-gov\.com)$`)

// yamlErrorHint maps a substring of a yaml.v3 error message to a human-readable
// description and fix suggestion.
//
// Note: go.yaml.in/yaml/v3 is a pure Go library and always emits English error
// messages regardless of the OS locale, so these substring matches are safe.
// However, they are tied to the library's internal wording and may need updating
// if the library changes its message format.
type yamlErrorHint struct {
	contains    string
	description string
	fix         string
}

var yamlErrorHints = []yamlErrorHint{
	{
		contains:    "found character that cannot start any token",
		description: "tab character used for indentation",
		fix:         "YAML requires spaces for indentation, not tabs — replace all tabs with spaces",
	},
	{
		contains:    "mapping values are not allowed",
		description: "incorrect indentation or missing space after colon",
		fix:         "check for missing spaces after colons or incorrect nesting",
	},
	{
		contains:    "did not find expected key",
		description: "unexpected indentation level",
		fix:         "a nested key may be at the wrong indentation level",
	},
}

// buildFriendlyYAMLError converts a raw yaml.v3 error into a human-readable
// message that includes the line number, the actual content of the offending
// line, and plain-English descriptions for all recognised error patterns.
// Returns a string because it always produces a message and never wraps another error.
func buildFriendlyYAMLError(yamlMsg string, lines []string) string {
	// Collect all matching hints so that compound errors are fully described.
	var descriptions, fixes []string
	for _, hint := range yamlErrorHints {
		if strings.Contains(yamlMsg, hint.contains) {
			descriptions = append(descriptions, hint.description)
			fixes = append(fixes, hint.fix)
		}
	}
	if len(descriptions) == 0 {
		descriptions = []string{yamlMsg}
		fixes = []string{"refer to the YAML error above for details"}
	}
	description := strings.Join(descriptions, "; ")
	fix := strings.Join(fixes, "; ")

	var lineNum int
	_, err := fmt.Sscanf(yamlMsg, "yaml: line %d:", &lineNum)
	if err != nil {
		return err.Error()
	}
	if lineNum > 0 && lineNum <= len(lines) {
		lineContent := strings.TrimRight(lines[lineNum-1], "\r")
		return fmt.Sprintf("YAML syntax error on line %d: %s\n  content: %q\n  fix: %s",
			lineNum, description, lineContent, fix)
	}
	return fmt.Sprintf("YAML syntax error: %s\n  fix: %s", description, fix)
}

// checkYAMLSyntax reads the config file at path, checks for tab characters, and
// validates that the file is parseable YAML. Returns (ok bool, warnings []string, err error):
// ok is false if the file could not be read or the YAML is structurally invalid,
// warnings lists non-fatal issues (e.g. tab indentation), and err gives context on failure.
func checkYAMLSyntax(path string) (bool, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil, fmt.Errorf("cannot read config file %s: %w", path, err)
	}

	var warnings []string
	lines := strings.Split(string(data), "\n")
	var tabLines []int
	for i, line := range lines {
		if strings.HasPrefix(line, "\t") {
			tabLines = append(tabLines, i+1)
		}
	}
	if len(tabLines) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"tab characters found on line(s) %s — YAML requires spaces for indentation",
			formatLineNumbers(tabLines),
		))
	}

	if err := yaml.Unmarshal(data, new(map[string]interface{})); err != nil {
		return false, warnings, errors.New(buildFriendlyYAMLError(err.Error(), lines))
	}

	return true, warnings, nil
}

// validateAPIKey validates the format of a Datadog API key.
// Returns (message, error): message is the human-readable result to be logged
// by the caller; error is non-nil only on a format violation.
// Empty keys and ENC[] keys are not errors.
// Uses HideKeyExceptLastFourChars from the agent's scrubber package
// for consistent API key masking.
func validateAPIKey(apiKey string) (message string, err error) {
	if apiKey == "" {
		// Empty may be valid with cloud-based authentication — warn, don't error.
		return "api_key not set — may be valid with cloud-based auth, but required for most configurations", nil
	}
	if scrubber.IsEnc(apiKey) {
		// ENC[] notation means the key is managed by a secret backend.
		return "api_key uses secret management (ENC[]), cannot validate format", nil
	}
	if !apiKeyRegex.MatchString(apiKey) {
		return "", fmt.Errorf("api_key format is invalid (got %d chars, expected 32 hex characters)", len(apiKey))
	}
	masked := scrubber.HideKeyExceptLastFourChars(apiKey)
	return fmt.Sprintf("api_key format is valid (%s)", masked), nil
}

// checkSite validates the configured site value. Returns the site string
// (defaulting to datadoghq.com if unset) and whether the site appears valid.
// Returns (valid, site, message) — message is for the caller to log.
func checkSite(cfg config.Component) (valid bool, site string, message string) {
	site = cfg.GetString("site")
	if site == "" {
		return true, "datadoghq.com", "no 'site' configured — defaulting to datadoghq.com (US1)"
	}
	if validSiteRe.MatchString(site) {
		return true, site, fmt.Sprintf("site '%s' is a valid Datadog site", site)
	}
	return false, site, fmt.Sprintf("site '%s' does not appear to be a valid Datadog site", site)
}

// checkAPIKeyLive calls the Datadog API to verify the key is accepted for the given site.
// Returns (ok bool, err error): ok is false only on a definitive rejection (HTTP 403);
// network errors and unexpected status codes are non-fatal and returned as errors with ok=true.
func checkAPIKeyLive(apiKey, site string) (bool, error) {
	baseURL := configutils.BuildURLWithPrefix("https://api.", site)
	url := baseURL + "/api/v1/validate"

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("could not build validation request: %w", err)
	}
	req.Header.Set("DD-API-KEY", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return true, fmt.Errorf("could not reach %s to validate API key: %w", baseURL, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusForbidden:
		return false, fmt.Errorf("API key rejected by %s (HTTP 403) — wrong key or wrong region. Verify your key matches this site", site)
	default:
		return true, fmt.Errorf("unexpected response from %s: HTTP %d", baseURL, resp.StatusCode)
	}
}

// checkEnabledProducts builds a product enablement summary.
// Returns (anyEnabled bool, lines []string) — lines are for the caller to log.
// Uses cfg.GetBool() directly rather than traversing the raw YAML map.
// Shows "no products" notice when none are enabled, then a single loop
// for both cases displaying [X] for disabled.
func checkEnabledProducts(cfg config.Component) (bool, []string) {
	products := []struct {
		key  string
		name string
	}{
		{"logs_enabled", "Log collection"},
		{"apm_config.enabled", "APM"},
		{"process_config.process_collection.enabled", "Live Process"},
		{"runtime_security_config.enabled", "Cloud Workload Security"},
		{"compliance_config.enabled", "Cloud Security Posture Management"},
		{"network_path.enabled", "Network Path"},
	}

	enabled := make([]bool, len(products))
	noneEnabled := true
	for i, p := range products {
		enabled[i] = cfg.GetBool(p.key)
		if enabled[i] {
			noneEnabled = false
		}
	}

	var lines []string
	lines = append(lines, "products: enabled product summary:")
	if noneEnabled {
		lines = append(lines, "         No products are enabled. Sending only Metrics.")
	}
	for i, p := range products {
		if enabled[i] {
			lines = append(lines, "         ✓  "+p.name)
		} else {
			lines = append(lines, "         [X] "+p.name+" (disabled)")
		}
	}
	return !noneEnabled, lines
}

func runConfigCheck(_ log.Component, cfg config.Component, noAPICheck bool) error {
	configPath := cfg.ConfigFileUsed()
	hasErrors := false
	out := os.Stdout

	// 1. File permissions (Unix: world-readable mode bits; Windows: Everyone ACE in DACL)
	if ok, err := checkFilePermissions(configPath); err != nil {
		fmt.Fprintf(out, "[WARN] permissions: %s\n", err)
	} else if ok {
		fmt.Fprintf(out, "[OK]   permissions: file permissions look good\n")
	}

	// 2. YAML syntax (also detects tabs)
	ok, yamlWarnings, err := checkYAMLSyntax(configPath)
	for _, w := range yamlWarnings {
		fmt.Fprintf(out, "[WARN] yaml_syntax: %s\n", w)
	}
	if !ok {
		fmt.Fprintf(out, "[ERR]  yaml_syntax: %s\n", err)
		return err // cannot continue without valid YAML
	}
	fmt.Fprintf(out, "[OK]   yaml_syntax: YAML syntax is valid\n")

	// 3. API key format — check function returns message; caller handles output
	apiKey := cfg.GetString("api_key")
	msg, err := validateAPIKey(apiKey)
	if err != nil {
		fmt.Fprintf(out, "[ERR]  %s\n", err)
		hasErrors = true
	} else if strings.Contains(msg, "not set") {
		fmt.Fprintf(out, "[WARN] %s\n", msg)
	} else {
		fmt.Fprintf(out, "[OK]   %s\n", msg)
	}

	// 4. Site validation — check function returns message; caller handles output
	siteValid, site, siteMsg := checkSite(cfg)
	if siteValid {
		fmt.Fprintf(out, "[OK]   site: %s\n", siteMsg)
	} else {
		fmt.Fprintf(out, "[ERR]  site: %s\n", siteMsg)
		hasErrors = true
	}

	// 5. Live API key validation (skip if --no-api, key missing/ENC[], or site invalid)
	canValidateLive := !noAPICheck && !hasErrors && siteValid && apiKey != "" && !scrubber.IsEnc(apiKey)
	if canValidateLive {
		if ok, err := checkAPIKeyLive(apiKey, site); err != nil {
			if ok {
				fmt.Fprintf(out, "[WARN] api_validate: %s\n", err)
			} else {
				fmt.Fprintf(out, "[ERR]  api_validate: %s\n", err)
				hasErrors = true
			}
		} else {
			fmt.Fprintf(out, "[OK]   api_validate: API key is valid for site %s\n", site)
		}
	}

	// 6. Product enablement summary — check function returns lines; caller handles output
	_, productLines := checkEnabledProducts(cfg)
	for _, line := range productLines {
		fmt.Fprintf(out, "%s\n", line)
	}

	if hasErrors {
		return errors.New("agent config check found errors — see output above")
	}
	return nil
}

func formatLineNumbers(lines []int) string {
	const maxShown = 5
	if len(lines) <= maxShown {
		parts := make([]string, len(lines))
		for i, l := range lines {
			parts[i] = strconv.Itoa(l)
		}
		return strings.Join(parts, ", ")
	}
	parts := make([]string, maxShown)
	for i := 0; i < maxShown; i++ {
		parts[i] = strconv.Itoa(lines[i])
	}
	return strings.Join(parts, ", ") + " (+" + strconv.Itoa(len(lines)-maxShown) + " more)"
}

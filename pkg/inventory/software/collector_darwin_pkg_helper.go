// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxTopLevelPathsPerPkg = 128

// topLevelToken stores a prefix-neutral top-level path candidate derived from one
// lsbom line. Relative tokens are anchored to the receipt prefix at read time.
type topLevelToken struct {
	Value    string
	Relative bool
}

// bomDigest stores compact derived data from one BOM payload, shared across
// both pkgReceiptsCollector and appToPkgIndex consumers.
type bomDigest struct {
	HasApplicationsApp bool
	HasNonAppPayload   bool
	TopLevelTokens     []topLevelToken
	AppPaths           []string // normalized (no "./"), relative to receipt prefix
}

// pkgSummary stores compact derived facts from lsbom directory listing output.
// It intentionally avoids retaining full file lists to reduce memory usage.
type pkgSummary struct {
	// HasApplicationsApp is true when pkg payload contains an app bundle under /Applications.
	HasApplicationsApp bool
	// HasNonAppPayload is true when pkg payload includes directories outside /Applications app bundles.
	HasNonAppPayload bool
	// TopLevelPaths stores deduplicated top-level install directories derived from pkg payload directory paths.
	TopLevelPaths []string
}

// pkgReceiptInfo holds parsed info from a PKG receipt plist.
type pkgReceiptInfo struct {
	packageID   string
	version     string
	installDate string
	prefixPath  string
	bomPath     string
}

// normalizeBomLine strips leading whitespace and the "./" prefix that lsbom
// emits, returning "" for blank lines or the bare "." root entry.
func normalizeBomLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "./")
	if line == "" || line == "." {
		return ""
	}
	return line
}

// normalizePrefixPath returns a normalized absolute install prefix, or "" for
// root ("/") and empty values.
func normalizePrefixPath(prefixPath string) string {
	if prefixPath == "" || prefixPath == "/" {
		return ""
	}
	if filepath.IsAbs(prefixPath) {
		return prefixPath
	}
	return "/" + prefixPath
}

// isApplicationsAppPath reports whether a pkg file path belongs to an app bundle in Applications.
// Inputs are normalized relative paths (no leading "./"), e.g. "Applications/Foo.app/Contents".
func isApplicationsAppPath(path string) bool {
	// Split into first component and remainder using the path separator.
	first, rest, _ := strings.Cut(path, string(filepath.Separator))
	// Case 1: prefix is "Applications", so path is relative to it — first component is the .app.
	// e.g. "Foo.app" or "Foo.app/Contents/MacOS"
	if filepath.Ext(first) == ".app" {
		return true
	}
	// Case 2: path is relative to "/", so "Applications" is the first component.
	// e.g. "Applications/Foo.app" or "Applications/Foo.app/Contents/MacOS"
	second, _, _ := strings.Cut(rest, string(filepath.Separator))
	return first == "Applications" && filepath.Ext(second) == ".app"
}

// topLevelTokenFromLine extracts the meaningful install root from a normalized
// lsbom line, applying per-directory depth rules for known macOS top-level dirs.
func topLevelTokenFromLine(line string) (topLevelToken, bool) {
	first, rest, _ := strings.Cut(line, string(filepath.Separator))
	if first == "" {
		return topLevelToken{}, false
	}

	switch first {
	case "usr":
		// Capture three levels: /usr/local/bin, /usr/local/lib, etc.
		second, rest2, _ := strings.Cut(rest, string(filepath.Separator))
		third, _, _ := strings.Cut(rest2, string(filepath.Separator))
		if second != "" && third != "" {
			return topLevelToken{Value: filepath.Join("/", first, second, third)}, true
		}
	case "Library", "opt", "Applications":
		// Capture two levels: /Library/LaunchDaemons, /opt/datadog-agent, /Applications/Foo.app
		second, _, _ := strings.Cut(rest, string(filepath.Separator))
		if second != "" {
			return topLevelToken{Value: filepath.Join("/", first, second)}, true
		}
	case "System", "private", "var":
		// Prefer three levels; fall back to two if the path is that shallow.
		second, rest2, _ := strings.Cut(rest, string(filepath.Separator))
		if second == "" {
			return topLevelToken{}, false
		}
		third, _, _ := strings.Cut(rest2, string(filepath.Separator))
		if third != "" {
			return topLevelToken{Value: filepath.Join("/", first, second, third)}, true
		}
		return topLevelToken{Value: filepath.Join("/", first, second)}, true
	default:
		// Unknown top-level dir — treat as relative to the receipt prefix.
		return topLevelToken{Value: first, Relative: true}, true
	}
	return topLevelToken{}, false
}

// topLevelPathFromToken resolves a token to an absolute path, anchoring relative
// tokens to the receipt's install prefix.
func topLevelPathFromToken(token topLevelToken, prefixPath string) string {
	if token.Value == "" {
		return ""
	}
	if !token.Relative {
		if token.Value == "/" {
			return ""
		}
		return token.Value
	}

	basePrefix := normalizePrefixPath(prefixPath)
	if basePrefix != "" {
		return filepath.Join(basePrefix, token.Value)
	}
	return filepath.Join("/", token.Value)
}

// bomDigestBuilder incrementally accumulates compact BOM digest facts while
// scanning lsbom output, avoiding full-line slice retention.
type bomDigestBuilder struct {
	digest       bomDigest
	seenTopLevel map[topLevelToken]struct{}
	seenAppPaths map[string]struct{}
}

func newBomDigestBuilder() *bomDigestBuilder {
	return &bomDigestBuilder{
		seenTopLevel: make(map[topLevelToken]struct{}),
		seenAppPaths: make(map[string]struct{}),
	}
}

func (b *bomDigestBuilder) addLine(line string) {
	line = normalizeBomLine(line)
	if line == "" {
		return
	}

	if isApplicationsAppPath(line) {
		b.digest.HasApplicationsApp = true
	} else {
		b.digest.HasNonAppPayload = true
	}

	if token, ok := topLevelTokenFromLine(line); ok {
		if _, exists := b.seenTopLevel[token]; !exists {
			b.seenTopLevel[token] = struct{}{}
			b.digest.TopLevelTokens = append(b.digest.TopLevelTokens, token)
		}
	}

	if filepath.Ext(line) == ".app" {
		if _, exists := b.seenAppPaths[line]; !exists {
			b.seenAppPaths[line] = struct{}{}
			b.digest.AppPaths = append(b.digest.AppPaths, line)
		}
	}
}

func (b *bomDigestBuilder) result() bomDigest {
	return b.digest
}

// buildBomDigest is a small helper that lets tests exercise the streaming builder.
func buildBomDigest(lines []string) bomDigest {
	builder := newBomDigestBuilder()
	for _, line := range lines {
		builder.addLine(line)
	}
	return builder.result()
}

// buildPkgSummaryFromDigest derives a pkgSummary from a pre-computed bomDigest,
// resolving relative top-level tokens against the receipt's install prefix.
func buildPkgSummaryFromDigest(digest bomDigest, prefixPath string) pkgSummary {
	summary := pkgSummary{
		HasApplicationsApp: digest.HasApplicationsApp,
		HasNonAppPayload:   digest.HasNonAppPayload,
	}
	topLevelSet := make(map[string]struct{})
	for _, token := range digest.TopLevelTokens {
		topLevelDir := topLevelPathFromToken(token, prefixPath)
		if topLevelDir == "" {
			continue
		}
		if len(topLevelSet) < maxTopLevelPathsPerPkg {
			topLevelSet[topLevelDir] = struct{}{}
		}
	}
	summary.TopLevelPaths = sortedPathsFromSet(topLevelSet)
	return summary
}

// buildPkgSummaryFromLines builds a compact package summary from lsbom -sd output lines.
// It remains as a thin helper so tests can assert summary behavior directly.
func buildPkgSummaryFromLines(lines []string, prefixPath string) pkgSummary {
	return buildPkgSummaryFromDigest(buildBomDigest(lines), prefixPath)
}

// sortedPathsFromSet converts a path set into a lexicographically sorted slice.
func sortedPathsFromSet(paths map[string]struct{}) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// shouldSkipPkgFromSummary returns true when a PKG receipt should be suppressed
// because its payload includes an .app bundle already captured by applicationsCollector.
func shouldSkipPkgFromSummary(summary pkgSummary) bool {
	return summary.HasApplicationsApp
}

// filterGenericSystemPaths removes overly generic install roots that carry no
// useful information about where a package installed its meaningful files.
func filterGenericSystemPaths(paths []string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "/etc" || path == "/var" || path == "/tmp" || path == "/System" {
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered
}

// buildEntryFromReceipt builds a software entry for one receipt using a pre-computed summary.
// Returns nil when the receipt should be skipped by representation rules.
func buildEntryFromReceipt(receipt pkgReceiptInfo, summary pkgSummary, is64Bit bool) *Entry {
	if shouldSkipPkgFromSummary(summary) {
		return nil
	}

	var installPath string
	if receipt.prefixPath != "" && receipt.prefixPath != "/" {
		installPath = normalizePrefixPath(receipt.prefixPath)
	} else {
		installPath = "N/A"
	}

	installPaths := filterGenericSystemPaths(summary.TopLevelPaths)

	if installPath != "N/A" && len(installPaths) > 0 {
		hasPathsOutside := false
		installPathWithSlash := installPath + "/"
		for _, p := range installPaths {
			if !strings.HasPrefix(p, installPathWithSlash) && p != installPath {
				hasPathsOutside = true
				break
			}
		}
		if !hasPathsOutside {
			installPaths = nil
		}
	} else if installPath == "N/A" && len(installPaths) > 0 {
		if len(installPaths) == 1 {
			installPath = installPaths[0]
			installPaths = nil
		} else {
			installPath = ""
		}
	}

	status := statusInstalled
	var brokenReason string
	if installPath != "" && installPath != "N/A" {
		if _, err := os.Stat(installPath); os.IsNotExist(err) {
			status = statusBroken
			brokenReason = "install path not found: " + installPath
		}
	} else if len(installPaths) > 0 {
		for _, p := range installPaths {
			if _, err := os.Stat(p); os.IsNotExist(err) {
				status = statusBroken
				brokenReason = "install path not found: " + p
				break
			}
		}
	}

	return &Entry{
		DisplayName:  receipt.packageID,
		Version:      receipt.version,
		InstallDate:  receipt.installDate,
		Source:       softwareTypePkg,
		ProductCode:  receipt.packageID,
		Status:       status,
		BrokenReason: brokenReason,
		Is64Bit:      is64Bit,
		InstallPath:  installPath,
		InstallPaths: installPaths,
	}
}

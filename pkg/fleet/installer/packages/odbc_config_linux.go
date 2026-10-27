// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	odbcDriver18Section = "ODBC Driver 18 for SQL Server"
	freeTDSSection      = "FreeTDS"
)

// ensureODBCDriverConfig writes embedded/etc/odbcinst.ini from the packaged
// embedded/share/odbc template, or repoints Driver paths that refer to another
// Datadog install, since Fleet Automation deletes /opt/datadog-agent after an
// upgrade. Drivers outside a Datadog install are left in place.
func ensureODBCDriverConfig(packagePath string) error {
	// stable and experiment are symlinks. Driver paths must use the versioned
	// directory so they keep pointing at this tree after the symlink moves.
	if resolved, err := filepath.EvalSymlinks(packagePath); err == nil {
		packagePath = resolved
	}
	msDriver := ""
	if matches, _ := filepath.Glob(filepath.Join(packagePath, "embedded", "msodbcsql", "lib64", "libmsodbcsql-*.so.*")); len(matches) > 0 {
		msDriver = matches[len(matches)-1]
	}
	tdsDriver := filepath.Join(packagePath, "embedded", "lib", "libtdsodbc.so")
	if !fileExists(tdsDriver) {
		tdsDriver = ""
	}
	if msDriver == "" && tdsDriver == "" {
		return nil
	}

	configPath := filepath.Join(packagePath, "embedded", "etc", "odbcinst.ini")
	content, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", configPath, err)
	}
	existing := string(content)
	if strings.TrimSpace(existing) == "" {
		templatePath := filepath.Join(packagePath, "embedded", "share", "odbc", "odbcinst.ini")
		content, err = os.ReadFile(templatePath)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", templatePath, err)
		}
	}
	updated := rewriteODBCInst(string(content), packagePath, msDriver, tdsDriver, fileExists)
	if updated == existing {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(configPath), err)
	}
	if err := os.WriteFile(configPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func rewriteODBCInst(content, packagePath, msDriver, tdsDriver string, exists func(string) bool) string {
	lines := strings.Split(content, "\n")
	section := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Like unixODBC, the section name ends at the first ']' and the rest
		// of the line is ignored.
		if strings.HasPrefix(trimmed, "[") {
			name := trimmed[1:]
			if end := strings.IndexByte(name, ']'); end >= 0 {
				name = name[:end]
			}
			section = strings.TrimSpace(name)
			continue
		}
		key, value, ok := splitINIValue(trimmed)
		if !ok || !strings.EqualFold(key, "Driver") {
			continue
		}
		replacement, known := driverLibraryForSection(section, msDriver, tdsDriver)
		if !known || !shouldRewriteDriver(value, packagePath, exists) {
			continue
		}
		lineEnding := ""
		if strings.HasSuffix(line, "\r") {
			lineEnding = "\r"
		}
		lines[i] = "Driver=" + replacement + lineEnding
	}
	return strings.Join(lines, "\n")
}

// driverLibraryForSection matches section names case-insensitively, like unixODBC.
func driverLibraryForSection(section, msDriver, tdsDriver string) (string, bool) {
	switch {
	case strings.EqualFold(section, odbcDriver18Section):
		return msDriver, msDriver != ""
	case strings.EqualFold(section, freeTDSSection):
		return tdsDriver, tdsDriver != ""
	default:
		return "", false
	}
}

func shouldRewriteDriver(driverPath, packagePath string, exists func(string) bool) bool {
	if driverPath == "" {
		return true
	}
	driverPath = filepath.Clean(driverPath)
	if isUnder(driverPath, packagePath) {
		return !exists(driverPath)
	}
	return isUnder(driverPath, "/opt/datadog-agent") || isUnder(driverPath, "/opt/datadog-packages/datadog-agent")
}

func isUnder(path, dir string) bool {
	return strings.HasPrefix(path, dir+"/")
}

func splitINIValue(line string) (string, string, bool) {
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return "", "", false
	}
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

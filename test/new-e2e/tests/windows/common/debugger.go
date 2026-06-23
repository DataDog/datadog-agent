// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	// CdbPath is the directory where the Debugging Tools for Windows are extracted
	CdbPath = "C:/debugtools"
	// CdbExe is the path to the cdb.exe console debugger
	CdbExe = "C:/debugtools/x64/cdb.exe"
	// CdbZipPath is the path where the debugtools zip file is downloaded
	CdbZipPath = "C:/debugtools.zip"
	// SymbolCachePath is the directory where downloaded debug symbols are cached
	SymbolCachePath = `C:\symbols`
	// MicrosoftSymbolURL is the Microsoft public symbol server, used to resolve OS symbols
	MicrosoftSymbolURL = "https://msdl.microsoft.com/download/symbols"
	// DatadogSymbolURL is the S3 bucket where Datadog driver PDB symbols (ddnpm, etc.) are uploaded
	DatadogSymbolURL = "https://s3.amazonaws.com/dd-windows-symbols/datadog-windows-filter"
	// AgentSymbolURL is the S3 bucket where Agent build pipeline PDB symbols
	// (agent.exe, trace-agent.exe, etc.) are uploaded, used to resolve user-mode
	// crash stacks in the Agent's own binaries
	AgentSymbolURL = "https://s3.amazonaws.com/dd-agent-mstesting/pipelines/windows-symbols"
)

// symbolEntries are the individual `srv*cache*server` entries that make up the
// symbol path, in priority order. They are kept as separate entries so they can be
// emitted as distinct `.sympath`/`.sympath+` lines in the cdb script (see
// analyzeScript): cdb treats `;` as a command separator, so a single
// `.sympath a;b;c` line is ambiguous, while one entry per line is not.
var symbolEntries = []string{
	fmt.Sprintf(`srv*%s*%s`, SymbolCachePath, MicrosoftSymbolURL),
	fmt.Sprintf(`srv*%s*%s`, SymbolCachePath, DatadogSymbolURL),
	fmt.Sprintf(`srv*%s*%s`, SymbolCachePath, AgentSymbolURL),
}

// DefaultSymbolPath is the default _NT_SYMBOL_PATH combining the Microsoft public
// symbol server, the Datadog driver symbol server, and the Agent build symbol
// server for full stack resolution in both kernel driver and Agent crashes.
var DefaultSymbolPath = strings.Join(symbolEntries, ";")

// analyzeScript builds the cdb command script run against a crash dump.
//
// The commands are fed to cdb via a script file (cdb -cf) rather than an inline
// `-c "..."` argument. Routing them through a file avoids two problems with the
// inline form:
//
//   - Quoting: the command string transits Go -> SSH -> the Windows default shell
//     (PowerShell) -> cdb. Embedded double quotes do not survive that round trip
//     intact, so the `-c "..."` payload arrives mangled and !analyze never runs.
//   - Semicolons: cdb treats `;` as a command separator, which collides with the
//     `;` used to join multiple symbol servers in a single `.sympath` argument.
//
// The script sets the symbol path explicitly (NOT .symfix, which resets to
// Microsoft-only and would drop the Datadog symbol servers), reloads symbols,
// runs verbose automated crash analysis, and quits.
func analyzeScript() string {
	lines := make([]string, 0, len(symbolEntries)+3)
	// First entry sets the path; subsequent entries append (.sympath+), so each
	// server lives on its own line and cdb's `;`-as-separator can't split them.
	for i, entry := range symbolEntries {
		if i == 0 {
			lines = append(lines, ".sympath "+entry)
		} else {
			lines = append(lines, ".sympath+ "+entry)
		}
	}
	lines = append(lines, ".reload", "!analyze -v", "q")
	// Use CRLF line endings for the script file written to the Windows host.
	return strings.Join(lines, "\r\n") + "\r\n"
}

// SetupCdb downloads and extracts the Debugging Tools for Windows (cdb.exe) to the remote host.
//
// cdb.exe is the command-line version of WinDbg. It is used to run !analyze -v on crash dumps
// non-interactively.
//
// This function downloads a pre-staged debugtools.zip from the artifact bucket (containing
// the Debugging Tools for Windows x64 directory from a Windows SDK installation) and
// configures the symbol path (DefaultSymbolPath) for automatic symbol resolution from the
// Microsoft public symbol server, the Datadog driver symbol server, and the Agent build
// symbol server.
func SetupCdb(host *components.RemoteHost) error {
	err := host.HostArtifactClient.Get("windows-products/debugtools.zip", CdbZipPath)
	if err != nil {
		return fmt.Errorf("failed to download debugtools: %w", err)
	}

	_, err = host.Execute(fmt.Sprintf(`if (-Not (Test-Path -Path '%s')) { Expand-Archive -Path '%s' -DestinationPath '%s' }`, CdbPath, CdbZipPath, CdbPath))
	if err != nil {
		return fmt.Errorf("failed to extract debugtools: %w", err)
	}

	// Verify cdb.exe was extracted
	_, err = host.Execute(fmt.Sprintf(`if (-Not (Test-Path -Path '%s')) { throw 'cdb.exe not found at %s after extraction' }`, CdbExe, CdbExe))
	if err != nil {
		return fmt.Errorf("cdb.exe not found after extraction: %w", err)
	}

	// Create symbol cache directory
	_, err = host.Execute(fmt.Sprintf(`New-Item -ItemType Directory -Path '%s' -Force`, SymbolCachePath))
	if err != nil {
		return fmt.Errorf("failed to create symbol cache directory: %w", err)
	}

	// Set _NT_SYMBOL_PATH so cdb.exe can resolve symbols from the Microsoft public
	// symbol server, the Datadog driver symbol server (ddnpm, ddprocmon, etc.), and
	// the Agent build symbol server (agent.exe, trace-agent.exe, etc.)
	_, err = host.Execute(fmt.Sprintf(`[Environment]::SetEnvironmentVariable('_NT_SYMBOL_PATH', '%s', 'Machine')`, DefaultSymbolPath))
	if err != nil {
		return fmt.Errorf("failed to set _NT_SYMBOL_PATH: %w", err)
	}

	return nil
}

// AnalyzeDump runs cdb.exe with !analyze -v on a crash dump file on the remote host
// and returns the analysis output.
//
// The dump can be either a user-mode dump (WER/procdump) or a kernel dump (MEMORY.DMP) —
// cdb.exe auto-detects the dump type.
//
// The first invocation may be slow as cdb downloads symbols from the Microsoft public
// symbol server and the Datadog driver symbol server. Subsequent runs use the cached
// symbols in C:\symbols.
func AnalyzeDump(host *components.RemoteHost, dumpPath string) (string, error) {
	// Verify cdb.exe is available
	_, err := host.Execute(fmt.Sprintf(`if (-Not (Test-Path -Path '%s')) { throw 'cdb.exe not found at %s. Run SetupCdb first.' }`, CdbExe, CdbExe))
	if err != nil {
		return "", fmt.Errorf("cdb.exe not found: %w", err)
	}

	// Write the cdb command script to a file on the host. We feed commands to cdb
	// via a script file (-cf) rather than an inline -c "..." argument because the
	// inline form's embedded double quotes and semicolons do not survive the
	// Go -> SSH -> PowerShell -> cdb round trip intact (see analyzeScript).
	//
	// host.WriteFile uses SFTP, so the script content is transferred byte-for-byte
	// with no shell quoting involved.
	tmpDir, err := host.GetTmpFolder()
	if err != nil {
		return "", fmt.Errorf("failed to get TMP folder: %w", err)
	}
	scriptPath := host.JoinPath(tmpDir, "cdb_analyze.txt")
	if _, err := host.WriteFile(scriptPath, []byte(analyzeScript())); err != nil {
		return "", fmt.Errorf("failed to write cdb script: %w", err)
	}
	defer func() {
		_, _ = host.Execute(fmt.Sprintf(`Remove-Item -Path '%s' -Force`, scriptPath))
	}()

	// Run cdb.exe non-interactively:
	// -z: open the dump file
	// -cf: read and execute commands from the script file
	//
	// The invocation contains only single-quoted path arguments (no embedded
	// double quotes, no semicolons), so it survives the SSH/PowerShell transport.
	cmd := fmt.Sprintf(`& '%s' -z '%s' -cf '%s'`, CdbExe, dumpPath, scriptPath)
	output, err := host.Execute(cmd)
	if err != nil {
		return output, fmt.Errorf("cdb analysis failed for %s: %w", dumpPath, err)
	}
	return output, nil
}

// AnalyzeAllWERDumps runs !analyze -v on all WER crash dumps in the given folder on the
// remote host. For each dump, the analysis output is:
//   - logged via t.Logf (appears in CI job logs)
//   - saved to a local file in localOutputDir as <host>-<dumpfile>-analysis.txt (artifact)
//
// This function continues analyzing dumps even if some fail, returning a joined error
// with all errors encountered.
func AnalyzeAllWERDumps(host *components.RemoteHost, dumpFolder string, localOutputDir string, t *testing.T) error {
	dumps, err := ListWERDumps(host, dumpFolder)
	if err != nil {
		return fmt.Errorf("failed to list WER dumps: %w", err)
	}

	var retErr error
	for _, dump := range dumps {
		output, err := AnalyzeDump(host, dump.Path)
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("analysis failed for %s: %w", dump.Path, err))
			if output != "" {
				t.Logf("Partial analysis output for %s:\n%s", dump.FileName, output)
			}
			continue
		}

		t.Logf("=== Crash dump analysis for %s ===\n%s", dump.FileName, output)

		// Save analysis output as an artifact
		analysisFileName := fmt.Sprintf("%s-%s-analysis.txt", host.Address, dump.FileName)
		analysisPath := filepath.Join(localOutputDir, analysisFileName)
		if writeErr := os.WriteFile(analysisPath, []byte(output), 0644); writeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed to write analysis for %s: %w", dump.Path, writeErr))
		}
	}

	return retErr
}

// AnalyzeKernelDump runs !analyze -v on a kernel crash dump (e.g. C:\Windows\MEMORY.DMP)
// on the remote host and returns the analysis output.
//
// Kernel dumps may be in a protected directory, so this function copies the dump to a
// temporary location before analysis (similar to DownloadSystemCrashDump).
func AnalyzeKernelDump(host *components.RemoteHost, dumpPath string) (string, error) {
	if exists, _ := host.FileExists(dumpPath); !exists {
		return "", fmt.Errorf("kernel dump not found at %s", dumpPath)
	}

	// Copy the dump to a temporary location since it may be in a protected directory
	tmpDir, err := host.GetTmpFolder()
	if err != nil {
		return "", fmt.Errorf("failed to get TMP folder: %w", err)
	}

	tmpPath := host.JoinPath(tmpDir, "analyze_dump.dmp")
	_, err = host.Execute(fmt.Sprintf(`Copy-Item -Path '%s' -Destination '%s'`, dumpPath, tmpPath))
	if err != nil {
		return "", fmt.Errorf("failed to copy kernel dump to temp location: %w", err)
	}

	output, err := AnalyzeDump(host, tmpPath)

	// Clean up the temporary copy
	_, _ = host.Execute(fmt.Sprintf(`Remove-Item -Path '%s' -Force`, tmpPath))

	return output, err
}

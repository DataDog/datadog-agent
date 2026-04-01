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
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

const (
	// CdbExe is the path to the cdb.exe console debugger after Windows SDK installation
	CdbExe = `C:\Program Files (x86)\Windows Kits\10\Debuggers\x64\cdb.exe`
	// WinSdkInstallerURL is the URL to download the Windows 11 SDK online installer
	WinSdkInstallerURL = "https://go.microsoft.com/fwlink/?linkid=2272610"
	// WinSdkInstallerPath is the path where the SDK installer is downloaded on the remote host
	WinSdkInstallerPath = `C:\winsdksetup.exe`
	// SymbolCachePath is the directory where downloaded debug symbols are cached
	SymbolCachePath = `C:\symbols`
	// DatadogSymbolURL is the S3 bucket where Datadog driver PDB symbols (ddnpm, etc.) are uploaded
	DatadogSymbolURL = "https://s3.amazonaws.com/dd-windows-symbols/datadog-windows-filter"
	// DefaultSymbolPath is the default _NT_SYMBOL_PATH combining Microsoft public symbols
	// and Datadog driver symbols for full stack resolution in kernel driver crashes
	DefaultSymbolPath = `srv*C:\symbols*https://msdl.microsoft.com/download/symbols;srv*C:\symbols*https://s3.amazonaws.com/dd-windows-symbols/datadog-windows-filter`
)

// SetupCdb downloads and installs the Debugging Tools for Windows (cdb.exe) on the remote host.
//
// cdb.exe is the command-line version of WinDbg. It is used to run !analyze -v on crash dumps
// non-interactively.
//
// This function downloads the Windows SDK online installer from Microsoft and installs only
// the "Debugging Tools for Windows" feature (~150MB), not the full SDK. The installer
// downloads components from the Microsoft CDN, so it requires internet access on the test VM.
func SetupCdb(host *components.RemoteHost) error {
	// Download the Windows SDK online installer
	err := DownloadFile(host, WinSdkInstallerURL, WinSdkInstallerPath)
	if err != nil {
		return fmt.Errorf("failed to download Windows SDK installer: %w", err)
	}

	// Install only the Debugging Tools for Windows feature
	// /features OptionId.WindowsDesktopDebuggers: install only the debugger tools
	// /quiet: silent install, no UI
	// /norestart: don't reboot
	_, err = host.Execute(
		fmt.Sprintf(`Start-Process -FilePath '%s' -ArgumentList '/features OptionId.WindowsDesktopDebuggers /quiet /norestart' -Wait -PassThru`, WinSdkInstallerPath),
	)
	if err != nil {
		return fmt.Errorf("failed to install Debugging Tools for Windows: %w", err)
	}

	// Verify cdb.exe was installed
	_, err = host.Execute(fmt.Sprintf(`if (-Not (Test-Path -Path '%s')) { throw 'cdb.exe not found after SDK install at %s' }`, CdbExe, CdbExe))
	if err != nil {
		return fmt.Errorf("cdb.exe not found after installation: %w", err)
	}

	// Create symbol cache directory
	_, err = host.Execute(fmt.Sprintf(`New-Item -ItemType Directory -Path '%s' -Force`, SymbolCachePath))
	if err != nil {
		return fmt.Errorf("failed to create symbol cache directory: %w", err)
	}

	// Set _NT_SYMBOL_PATH so cdb.exe can resolve symbols from the Microsoft public symbol server
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
// symbol server. Subsequent runs use the cached symbols in C:\symbols.
func AnalyzeDump(host *components.RemoteHost, dumpPath string) (string, error) {
	// Verify cdb.exe is available
	_, err := host.Execute(fmt.Sprintf(`if (-Not (Test-Path -Path '%s')) { throw 'cdb.exe not found at %s. Run SetupCdb first.' }`, CdbExe, CdbExe))
	if err != nil {
		return "", fmt.Errorf("cdb.exe not found: %w", err)
	}

	// Run cdb.exe non-interactively:
	// -z: open the dump file
	// -c: execute commands then quit
	//   .symfix: configure the default Microsoft symbol server path
	//   .reload: reload symbols
	//   !analyze -v: verbose automated crash analysis
	//   q: quit cdb
	cmd := fmt.Sprintf(`& '%s' -z '%s' -c ".symfix; .reload; !analyze -v; q"`, CdbExe, dumpPath)
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

// GenerateCrashDump creates a deliberate crash on the remote host to produce a WER dump.
//
// It compiles a small C# console application that dereferences a null pointer, causing an
// access violation. WER catches the unhandled exception and writes a full dump to dumpFolder
// (as configured by EnableWERGlobalDumps).
//
// This is intended for validating the crash dump analysis pipeline in tests.
func GenerateCrashDump(host *components.RemoteHost, dumpFolder string) error {
	// Compile a tiny crashing executable on the remote host
	_, err := host.Execute(`
Add-Type -OutputAssembly 'C:\crashme.exe' -OutputType ConsoleApplication -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
class Program {
    static void Main() {
        Marshal.ReadInt32(IntPtr.Zero);
    }
}
'@
`)
	if err != nil {
		return fmt.Errorf("failed to compile crash program: %w", err)
	}

	// Run the crashing program. It will exit with an access violation;
	// WER will generate a dump in dumpFolder.
	// We use Start-Process -Wait so PowerShell doesn't treat the non-zero exit as an error
	// before WER has a chance to write the dump.
	_, _ = host.Execute(`Start-Process -FilePath 'C:\crashme.exe' -Wait -ErrorAction SilentlyContinue`)

	// Give WER time to finish writing the dump file
	_, err = host.Execute(fmt.Sprintf(`
$deadline = (Get-Date).AddSeconds(30)
while ((Get-Date) -lt $deadline) {
    $dumps = Get-ChildItem -Path '%s' -Filter '*.dmp' -ErrorAction SilentlyContinue
    if ($dumps.Count -gt 0) { break }
    Start-Sleep -Seconds 1
}
if (-not $dumps -or $dumps.Count -eq 0) {
    throw 'No crash dump was generated in %s after 30 seconds'
}
`, dumpFolder, dumpFolder))
	if err != nil {
		return fmt.Errorf("crash dump was not generated: %w", err)
	}

	return nil
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Tripwire test added by vuln-hunt campaign 2026-05-18-initial-audit /
// signal-handling.
//
// Invariant (Appendix A of the vuln-hunt skill): "Signals do not leave the
// interpreter in an inconsistent state; trap handlers cannot bypass the
// sandbox." The only way for an interpreter or builtin to install an OS
// signal handler is to import os/signal and call signal.Notify (or one of
// its siblings). rshell does not, by design: there is no orderly shutdown
// path, no SIGINT trap, no SIGPIPE override, no NotifyContext wrapper.
//
// The static symbol allowlist is the load-bearing enforcement: any new
// symbol imported by interp/ or builtins/ must be added explicitly to
// interpAllowedSymbols or builtinAllowedSymbols. This test pins the
// absence of every os/signal symbol from those lists so a future
// contributor cannot quietly add one without tripping a verification
// failure that points back at this audit.

package analysis

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// signalRelatedSymbols is the closed set of os/signal exports that, if
// allowed, would re-introduce in-process signal handling and the trap-
// handler attack surface called out in the vuln-hunt threat model.
var signalRelatedSymbols = []string{
	"os/signal.Notify",
	"os/signal.NotifyContext",
	"os/signal.Stop",
	"os/signal.Reset",
	"os/signal.Ignore",
	"os/signal.Ignored",
}

// TestVulnHuntSubsystemSignalHandling_NoOsSignalSymbolsInAllowlists asserts
// that no os/signal export is reachable from the interpreter core or any
// builtin via the symbol allowlists. The test is intentionally explicit
// (rather than relying on "absence of an unlisted symbol is caught at run
// time") so that the prohibition is documented in code and any attempt to
// allow os/signal fails this test before reaching review.
func TestVulnHuntSubsystemSignalHandling_NoOsSignalSymbolsInAllowlists(t *testing.T) {
	check := func(t *testing.T, listName string, list []string) {
		t.Helper()
		for _, sym := range list {
			if strings.HasPrefix(sym, "os/signal.") {
				t.Errorf("%s contains %q: rshell installs no OS signal handlers by design; "+
					"adding os/signal symbols re-introduces the trap-handler attack surface. "+
					"If this is intentional, update the signal-handling subsystem audit "+
					"(vuln-hunt 2026-05-18-initial-audit) and remove this test.", listName, sym)
			}
		}
	}

	check(t, "interpAllowedSymbols", interpAllowedSymbols)
	check(t, "builtinAllowedSymbols", builtinAllowedSymbols)
	check(t, "allowedpathsAllowedSymbols", allowedpathsAllowedSymbols)
	check(t, "fsstatAllowedSymbols", fsstatAllowedSymbols)

	// Per-command and per-internal-package maps are nested. Walk every entry.
	for cmd, syms := range builtinPerCommandSymbols {
		check(t, "builtinPerCommandSymbols["+cmd+"]", syms)
	}
	for pkg, syms := range internalPerPackageSymbols {
		check(t, "internalPerPackageSymbols["+pkg+"]", syms)
	}
}

// TestVulnHuntSubsystemSignalHandling_NoSignalNotifyInProductionCode is a
// belt-and-braces grep-style check that no non-test .go file under production
// directories contains the literal text "signal.Notify" or imports
// "os/signal". The symbol allowlist already enforces this. The collector gap
// that originally motivated this fallback (vuln-hunt F-1: top-level builtins/
// files bypassing the allowlist) has since been closed, so this test now
// serves purely as defense-in-depth against future regressions.
//
// Note: the function name slug uses "OsSignal" rather than the more
// natural "os/signal" because slashes in test names cause subtest pathing
// issues with `go test -run`.
func TestVulnHuntSubsystemSignalHandling_NoSignalNotifyInProductionCode(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{"allowedpaths", "builtins", "cmd", "interp"} {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			contents := string(data)
			if strings.Contains(contents, `"os/signal"`) {
				t.Errorf("%s imports os/signal: this re-opens the trap-handler attack "+
					"surface that the signal-handling vuln-hunt audit explicitly "+
					"closed. See vuln-hunt 2026-05-18-initial-audit / signal-handling.",
					path)
			}
			if strings.Contains(contents, "signal.Notify") ||
				strings.Contains(contents, "signal.NotifyContext") {
				t.Errorf("%s references signal.Notify[Context]: rshell installs no "+
					"OS signal handlers by design. See vuln-hunt 2026-05-18-initial-audit "+
					"/ signal-handling.", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
}

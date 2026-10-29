// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package help implements the help builtin command.
//
// help — display help for rshell features and commands
//
// Usage: help [--all] [feature|command]
//
// With no arguments, list rshell features with descriptions, a concise
// unsupported-feature summary, commands available in the current mode,
// allowlisted commands that require remediation mode, a compact list of
// commands disabled by policy, commands that may be prefixed with sudo, the
// configured AllowedPaths sandbox roots grouped by access mode, and the
// effective systemd unit/action grants (with explicit notices when elevation,
// path, or systemd policies are empty). When --all is given, commands disabled
// by policy are shown as a full description table.
// When a feature or command name is given, display detailed help for that topic.
//
// Flags:
//
//	--all   show commands disabled by policy with descriptions
//
// Exit codes:
//
//	0  Success (including when --help was requested).
//	1  Unknown topic.
package help

import (
	"bytes"
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/version"
)

// Cmd is the help builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "help",
	Description: "display help for features and commands",
	MakeFlags:   registerFlags,
}

func printUsage(callCtx *builtins.CallContext) {
	callCtx.Out("Usage: help [--all] [feature|command]\n")
	callCtx.Out("Display help for rshell features and commands.\n")
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	helpFlag := fs.Bool("help", false, "print usage and exit")
	allFlag := fs.Bool("all", false, "show commands disabled by policy with descriptions; ignored when a topic is given")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *helpFlag {
			printUsage(callCtx)
			return builtins.Result{}
		}

		// help <feature|command> — show detailed help for a specific topic.
		if len(args) > 1 {
			printUsage(callCtx)
			return builtins.Result{Code: 1}
		}
		if len(args) > 0 {
			name := args[0]
			if feature, ok := builtins.Feature(name); ok {
				printFeatureDetails(callCtx, feature)
				return builtins.Result{}
			}
			if callCtx.CommandAllowed != nil && !callCtx.CommandAllowed(name) {
				callCtx.Errf("help: no help topics match '%s'\n", builtins.SafeOperand(name))
				return builtins.Result{Code: 1}
			}
			meta, ok := builtins.Meta(name)
			if !ok {
				callCtx.Errf("help: no help topics match '%s'\n", builtins.SafeOperand(name))
				return builtins.Result{Code: 1}
			}

			// Use static Help text if available (for commands that don't
			// handle --help, like echo, true, false).
			if meta.Help != "" {
				callCtx.Outf("%s\n", meta.Help)
				return builtins.Result{}
			}

			// RemediationOnly builtins must never have their handler invoked
			// outside remediation mode, even via `help <name> --help`-style
			// lookup below. This mirrors the dispatch-level gate in interp so
			// a builtin that forgets its own remediation check cannot still
			// run its handler (and leak whatever it writes) through help.
			if meta.RemediationOnly && !callCtx.RemediationMode {
				msg := meta.RemediationDeniedMessage
				if msg == "" {
					msg = builtins.DefaultRemediationDeniedMessage(name)
				}
				callCtx.Errf("%s", msg)
				return builtins.Result{Code: 1}
			}

			// Otherwise, invoke the command with --help and capture the output.
			if handler, ok := builtins.Lookup(name); ok && handler != nil {
				var buf bytes.Buffer
				captureCtx := *callCtx
				captureCtx.Stdout = &buf
				captureCtx.Stderr = &buf
				handler(ctx, &captureCtx, []string{"--help"})
				if buf.Len() > 0 {
					callCtx.Outf("%s", buf.String())
					return builtins.Result{}
				}
			}

			callCtx.Outf("%s - %s\n", meta.Name, meta.Description)
			return builtins.Result{}
		}

		// No arguments — list features and commands.
		var available, requiresRemediation, disabledByPolicy []string
		for _, name := range builtins.Names() {
			if callCtx.CommandAllowed != nil && !callCtx.CommandAllowed(name) {
				disabledByPolicy = append(disabledByPolicy, name)
				continue
			}
			meta, _ := builtins.Meta(name)
			if meta.RemediationOnly && !callCtx.RemediationMode {
				requiresRemediation = append(requiresRemediation, name)
				continue
			}
			available = append(available, name)
		}

		// Header: version line only.
		printHeader(callCtx)

		callCtx.Out("Features:\n")
		printFeatureTable(callCtx, builtins.Features())
		printUnsupportedSummary(callCtx, builtins.UnsupportedSummary())

		callCtx.Outf("\nCommands available now (%d):\n", len(available))
		printCommandTable(callCtx, available)

		// Mode-gated commands always include descriptions, even without --all.
		if len(requiresRemediation) > 0 {
			callCtx.Outf("\nRequire remediation mode (%d):\n", len(requiresRemediation))
			printCommandTable(callCtx, requiresRemediation)
		}

		if len(disabledByPolicy) > 0 {
			if *allFlag {
				callCtx.Outf("\nDisabled by policy (%d):\n", len(disabledByPolicy))
				printCommandTable(callCtx, disabledByPolicy)
			} else {
				callCtx.Outf("\nDisabled by policy (%d): %s\n", len(disabledByPolicy), wrapNames(disabledByPolicy, 80))
			}
		} else if *allFlag && len(requiresRemediation) == 0 {
			callCtx.Out("\nAll commands are allowed in this session.\n")
		}

		printElevatableCommands(callCtx, available)
		printAllowedPaths(callCtx)
		printAllowedSystemServices(callCtx)

		callCtx.Out("\nRun 'help <feature|command>' for more information on a specific topic.\n")
		return builtins.Result{}
	}
}

// printElevatableCommands writes the effective commands that accept rshell's
// sudo marker. ElevatableCommandsList only reflects AllowedCommands (via the
// runner's r.allowAllCommands || r.allowedCommands check), not whether the
// command is actually runnable in the current mode — a RemediationOnly
// command (e.g. rm, truncate, logrotate, systemctl) can be allowed yet still
// require remediation mode. Filter against the already-computed "available
// now" set so this section never lists a command that isn't in
// "Commands available now" above.
func printElevatableCommands(callCtx *builtins.CallContext, available []string) {
	if callCtx.ElevatableCommandsList == nil {
		return
	}
	commands := callCtx.ElevatableCommandsList()
	availableSet := make(map[string]bool, len(available))
	for _, name := range available {
		availableSet[name] = true
	}
	var filtered []string
	for _, command := range commands {
		if availableSet[command] {
			filtered = append(filtered, command)
		}
	}
	callCtx.Out("\nElevatable commands:\n")
	if len(filtered) == 0 {
		callCtx.Out("  (no effective elevatable commands — sudo is unavailable)\n")
		return
	}
	for _, command := range filtered {
		callCtx.Outf("  sudo %s\n", command)
	}
}

// printHeader writes the version line with the current shell mode.
func printHeader(callCtx *builtins.CallContext) {
	mode := "read-only mode"
	if callCtx.RemediationMode {
		mode = "remediation mode"
	}
	if version.Version != "" {
		callCtx.Outf("rshell (%s) - %s\n\n", version.Version, mode)
	} else {
		callCtx.Outf("rshell - %s\n\n", mode)
	}
}

// printFeatureTable prints an aligned feature name/description table.
func printFeatureTable(callCtx *builtins.CallContext, features []builtins.FeatureMeta) {
	maxLen := 0
	for _, feature := range features {
		if len(feature.Name) > maxLen {
			maxLen = len(feature.Name)
		}
	}
	for _, feature := range features {
		callCtx.Outf("%-*s  %s\n", maxLen, feature.Name, feature.Description)
	}
}

// printAllowedPaths writes the configured AllowedPaths sandbox roots grouped
// by configured access mode. An empty list means no allowed paths are
// configured, which blocks all user-controllable filesystem access — surface
// that explicitly so operators can tell the difference from "no information
// available". A few builtins (ss, ip route, df, ps) intentionally read
// kernel-state paths outside the sandbox and are unaffected by this
// configuration.
func printAllowedPaths(callCtx *builtins.CallContext) {
	if callCtx.AllowedPathsList == nil {
		return
	}
	paths := callCtx.AllowedPathsList()
	callCtx.Out("\nAllowed paths:\n")
	if len(paths) == 0 {
		callCtx.Out("  (no allowed paths configured — no filesystem paths are reachable)\n")
		return
	}
	var readOnly, readWrite []string
	for _, p := range paths {
		if p.Access == builtins.AllowedPathReadWrite {
			readWrite = append(readWrite, p.Path)
			continue
		}
		readOnly = append(readOnly, p.Path)
	}
	printAllowedPathGroup(callCtx, "Read-only", readOnly)
	printAllowedPathGroup(callCtx, "Read-write", readWrite)
	if len(readWrite) > 0 && !callCtx.RemediationMode {
		callCtx.Out("  (write access requires remediation mode)\n")
	}
	if len(readWrite) == 0 && callCtx.RemediationMode {
		callCtx.Out("  (no read-write path configured — remediation commands cannot modify any file)\n")
	}
}

// printAllowedSystemServices writes the effective, validated
// AllowedSystemServices grants in the same UNIT:ACTION[+ACTION...] form
// accepted by the development CLI. The runner returns operations grouped by
// sorted exact unit name and canonical action order.
func printAllowedSystemServices(callCtx *builtins.CallContext) {
	if callCtx.AllowedSystemServicesList == nil {
		return
	}
	operations := callCtx.AllowedSystemServicesList()
	callCtx.Out("\nAllowed systemd units:\n")
	if len(operations) == 0 {
		callCtx.Out("  (no effective systemd unit grants — all systemd operations are blocked)\n")
		return
	}

	for i := 0; i < len(operations); {
		service := operations[i].Service
		callCtx.Outf("  %s:%s", service, operations[i].Action)
		i++
		for i < len(operations) && operations[i].Service == service {
			callCtx.Outf("+%s", operations[i].Action)
			i++
		}
		callCtx.Out("\n")
	}
	if !callCtx.RemediationMode {
		callCtx.Out("  (systemctl requires remediation mode; non-read actions are inactive in read-only mode)\n")
	}
}

func printAllowedPathGroup(callCtx *builtins.CallContext, label string, paths []string) {
	callCtx.Outf("  %s:\n", label)
	if len(paths) == 0 {
		callCtx.Out("    (none)\n")
		return
	}
	for _, path := range paths {
		callCtx.Outf("    %s\n", path)
	}
}

func printUnsupportedSummary(callCtx *builtins.CallContext, items []string) {
	if len(items) == 0 {
		return
	}
	callCtx.Out("\nNot supported:\n")
	for _, item := range items {
		callCtx.Outf("  - %s\n", item)
	}
}

func printFeatureDetails(callCtx *builtins.CallContext, feature builtins.FeatureMeta) {
	callCtx.Outf("%s - %s\n", feature.Name, feature.Description)
	printBulletSection(callCtx, "Supported", feature.Supported)
	printBulletSection(callCtx, "Not supported", feature.Unsupported)
	printBulletSection(callCtx, "Notes", feature.Notes)
}

func printBulletSection(callCtx *builtins.CallContext, title string, items []string) {
	if len(items) == 0 {
		return
	}
	callCtx.Outf("\n%s:\n", title)
	for _, item := range items {
		callCtx.Outf("  - %s\n", item)
	}
}

// printCommandTable prints an aligned name/description table.
func printCommandTable(callCtx *builtins.CallContext, names []string) {
	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	for _, name := range names {
		meta, _ := builtins.Meta(name)
		callCtx.Outf("%-*s  %s\n", maxLen, name, meta.Description)
	}
}

// wrapNames formats a list of names into comma-separated lines that stay
// within the given line width. Continuation lines are indented by two spaces.
func wrapNames(names []string, lineWidth int) string {
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	col := 0
	lineStart := true // true at the very start and after each newline indent
	for i, name := range names {
		token := name
		if i < len(names)-1 {
			token += ","
		}
		needed := len(token)
		if !lineStart {
			needed++ // space before token
		}
		if !lineStart && col+needed > lineWidth {
			b.WriteString("\n  ")
			col = 2
			lineStart = true
		}
		if !lineStart {
			b.WriteByte(' ')
			col++
		}
		b.WriteString(token)
		col += len(token)
		lineStart = false
	}
	return b.String()
}

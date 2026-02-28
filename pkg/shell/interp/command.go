// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/shell/verifier"
)

// safePATH is the controlled set of directories used to resolve external commands.
// We never use the caller's PATH to prevent PATH manipulation attacks.
var safePATH = []string{"/usr/bin", "/bin", "/usr/local/bin"}

// execCommand validates and executes an external command.
func (r *Runner) execCommand(ctx context.Context, name string, args []string) error {
	// Check for blocked builtins.
	if verifier.IsBlockedBuiltin(name) {
		return fmt.Errorf("command %q is not allowed", name)
	}

	// Check the command is in the allowlist.
	allowedFlags, ok := verifier.AllowedCommandFlags(name)
	if !ok {
		return fmt.Errorf("command %q is not allowed", name)
	}

	// Validate flags after expansion.
	if err := validateFlags(name, args, allowedFlags); err != nil {
		return err
	}

	// Special handling for sed: validate scripts.
	if name == "sed" {
		if err := verifier.ValidateSedArgs(args); err != nil {
			return err
		}
	}

	// Resolve to absolute path using safe PATH.
	absPath, err := resolveCommand(name)
	if err != nil {
		r.exitCode = 127
		return err
	}

	cmd := exec.CommandContext(ctx, absPath, args...)
	cmd.Dir = r.dir
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	cmd.Env = r.env

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			r.exitCode = exitErr.ExitCode()
			return nil
		}
		return fmt.Errorf("execution failed: %w", err)
	}

	r.exitCode = 0
	return nil
}

// validateFlags checks that all flags in args are in the allowlist for the given command.
func validateFlags(cmdName string, args []string, allowed map[string]bool) error {
	for _, arg := range args {
		// Skip non-flag arguments.
		if !strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "+") {
			continue
		}

		// Handle + prefixed flags.
		if strings.HasPrefix(arg, "+") {
			if !allowed[arg] {
				return fmt.Errorf("flag %q is not allowed for command %q", arg, cmdName)
			}
			continue
		}

		// Skip "--" itself but keep validating remaining args.
		// We do NOT return early because some commands (e.g. find) parse
		// expression arguments like -exec even after --.
		if arg == "--" {
			continue
		}

		// Handle long flags (--foo or --foo=bar).
		if strings.HasPrefix(arg, "--") {
			flagName := arg
			if idx := strings.Index(arg, "="); idx != -1 {
				flagName = arg[:idx]
			}
			if !allowed[flagName] {
				return fmt.Errorf("flag %q is not allowed for command %q", flagName, cmdName)
			}
			continue
		}

		// Check if the whole flag is known.
		if allowed[arg] {
			continue
		}

		// Check combined short flags character by character.
		for i := 1; i < len(arg); i++ {
			singleFlag := "-" + string(arg[i])
			if !allowed[singleFlag] {
				if arg[i] >= '0' && arg[i] <= '9' {
					allDigits := true
					for j := i; j < len(arg); j++ {
						if arg[j] < '0' || arg[j] > '9' {
							allDigits = false
							break
						}
					}
					if allDigits {
						break
					}
				}
				return fmt.Errorf("flag %q is not allowed for command %q", singleFlag, cmdName)
			}
		}
	}
	return nil
}

// resolveCommand resolves a command name to an absolute path using only the safe PATH.
func resolveCommand(name string) (string, error) {
	for _, dir := range safePATH {
		path := dir + "/" + name
		if isExecutable(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("command %q not found in safe PATH", name)
}

// isExecutable checks if a file exists and is executable.
func isExecutable(path string) bool {
	_, err := exec.LookPath(path)
	return err == nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main provides the rshell development CLI. It is a local harness for
// exercising the interpreter, not the production security boundary; production
// integrations should embed the interp package and configure policy explicitly.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/version"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
	"github.com/spf13/cobra"
)

const exitCodeTimeout = 124

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "privileged-worker" {
		// The worker is an internal, one-shot child of privileged-helper. Keep it
		// free of telemetry goroutines and any other concurrent work before its
		// process-wide sandbox is installed.
		os.Exit(runPrivilegedWorker(context.Background(), args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	if len(args) > 0 && args[0] == "privileged-helper" {
		// Keep the privileged process free of telemetry goroutines: seteuid is
		// process-wide, so unrelated concurrent work must not overlap elevation.
		os.Exit(runPrivilegedHelper(context.Background(), args[1:], os.Stderr))
	}
	stopTelemetry := startTelemetry()
	code := run(context.Background(), args, os.Stdin, os.Stdout, os.Stderr)
	// Flush before os.Exit — os.Exit does not run deferred calls.
	stopTelemetry()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var (
		command                  string
		allowedPaths             string
		allowedCommands          string
		allowedServices          string
		allowAllCmds             bool
		timeout                  time.Duration
		procPath                 string
		journalDirs              string
		machineIDPath            string
		journalSocket            string
		managerSocket            string
		mode                     string
		disableDetailedTelemetry bool
	)

	cmd := &cobra.Command{
		Use:           "rshell [file ...]",
		Short:         "A restricted shell interpreter for AI agents",
		Long:          "rshell is a development, debugging, and local validation harness for the interpreter. Production integrations should embed the Go API and configure commands, paths, environment, timeout, and mode explicitly.",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		// Reject the hidden --command long form: -c is short-only (bash convention).
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return rejectLongCommand(args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			commandSet := cmd.Flags().Changed("command")
			if commandSet && len(args) > 0 {
				return errors.New("cannot use -c with file arguments")
			}

			if timeout < 0 {
				return errors.New("--timeout must be >= 0")
			}

			runCtx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(runCtx, timeout)
				defer cancel()
			}

			var paths []string
			if allowedPaths != "" {
				paths = strings.Split(allowedPaths, ",")
			}

			var cmds []string
			if allowedCommands != "" {
				cmds = strings.Split(allowedCommands, ",")
			}

			serviceGrants := parseAllowedServices(allowedServices)
			parsedMode := interp.Mode(mode)
			if parsedMode != interp.ModeReadOnly && parsedMode != interp.ModeRemediation {
				return errors.New("--mode must be one of: read-only, remediation")
			}

			var configuredJournalDirs []string
			if journalDirs != "" {
				configuredJournalDirs = strings.Split(journalDirs, ",")
			}
			systemdTargetSet := journalDirs != "" || machineIDPath != "" || journalSocket != "" || managerSocket != ""

			execOpts := executeOpts{
				allowedPaths:     paths,
				allowedCommands:  cmds,
				allowedServices:  serviceGrants,
				allowAllCommands: allowAllCmds,
				procPath:         procPath,
				systemdTarget: interp.SystemdTargetConfig{
					JournalDirs:          configuredJournalDirs,
					MachineIDPath:        machineIDPath,
					JournalControlSocket: journalSocket,
					ManagerBusSocket:     managerSocket,
				},
				systemdTargetSet:         systemdTargetSet,
				mode:                     parsedMode,
				disableDetailedTelemetry: disableDetailedTelemetry,
			}

			if commandSet {
				return execute(runCtx, command, "", execOpts, stdin, stdout, stderr)
			}

			if len(args) > 0 {
				// Read stdin once so each execute() call gets its own
				// reader, avoiding a data race on the shared io.Reader.
				stdinData, err := readAllContext(runCtx, stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}

				for _, file := range args {
					f, err := os.Open(file)
					if err != nil {
						return fmt.Errorf("reading %s: %w", file, err)
					}
					data, err := readAllContext(runCtx, f)
					f.Close()
					if err != nil {
						return fmt.Errorf("reading %s: %w", file, err)
					}
					if err := execute(runCtx, string(data), file, execOpts, bytes.NewReader(stdinData), stdout, stderr); err != nil {
						return err
					}
				}
				return nil
			}

			// No -c and no file args: read from stdin.
			stdinData, err := readAllContext(runCtx, stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			return execute(runCtx, string(stdinData), "", execOpts, strings.NewReader(""), stdout, stderr)
		},
	}

	cmd.SetArgs(args)
	cmd.SetIn(stdin)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cmd.Flags().StringVarP(&command, "command", "c", "", "shell command string to execute")
	cmd.Flags().MarkHidden("command") //nolint:errcheck // flag is guaranteed to exist
	cmd.Flags().StringVarP(&allowedPaths, "allowed-paths", "p", "", "comma-separated list of PATH[:ro|:rw] directories the shell is allowed to access; entries without a suffix are read-only")
	cmd.Flags().StringVar(&allowedCommands, "allowed-commands", "", "comma-separated list of namespaced commands (e.g. rshell:cat,rshell:find)")
	cmd.Flags().StringVar(&allowedServices, "allowed-services", "", "comma-separated systemd unit grants in UNIT:ACTION[+ACTION...] or UNIT:* form")
	cmd.Flags().BoolVar(&allowAllCmds, "allow-all-commands", false, "allow execution of all commands (builtins and external)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "maximum execution time for the entire shell run (e.g. 100ms, 5s, 1m)")
	cmd.Flags().StringVar(&procPath, "proc-path", "", "path to the proc filesystem used by ps (default \"/proc\")")
	cmd.Flags().StringVar(&journalDirs, "systemd-journal-dirs", "", "comma-separated journal root directories for an explicit systemd target")
	cmd.Flags().StringVar(&machineIDPath, "systemd-machine-id-path", "", "machine-id file for an explicit systemd target")
	cmd.Flags().StringVar(&journalSocket, "systemd-journal-socket", "", "journald Varlink socket for an explicit systemd target")
	cmd.Flags().StringVar(&managerSocket, "systemd-manager-socket", "", "system D-Bus socket for an explicit systemd target")
	cmd.Flags().StringVar(&mode, "mode", "read-only", "shell execution mode: read-only (default) or remediation (enables file-target output redirections within :rw AllowedPaths roots and remediation-only builtins, including the restricted systemctl builtin)")
	cmd.Flags().BoolVar(&disableDetailedTelemetry, "disable-detailed-telemetry", false, "suppress the rshell.run.command and rshell.run.options.* tags on the top-level run telemetry span (on by default; set this when the raw command or effective sandbox configuration is too sensitive to report)")

	if err := cmd.ExecuteContext(ctx); err != nil {
		var status interp.ExitStatus
		if errors.As(err, &status) {
			return int(status)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			if timeout > 0 {
				fmt.Fprintf(stderr, "error: execution timed out after %s\n", timeout)
			} else {
				fmt.Fprintln(stderr, "error: execution timed out")
			}
			return exitCodeTimeout
		}
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, "error: execution canceled")
			return exitCodeTimeout
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// readAllContext reads all bytes from r, but returns ctx.Err() immediately if
// the context is cancelled or its deadline expires before the read completes.
// It spawns a goroutine to perform the read; the goroutine may outlive this
// call if the underlying reader blocks (e.g. stdin from a pipe), but it will
// be reclaimed when the process exits.
func readAllContext(ctx context.Context, r io.Reader) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		ch <- result{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.data, res.err
	}
}

// rejectLongCommand scans raw CLI args for "--command" or "--command=..." and
// returns an error if found. The flag is registered with a long name so that
// cobra/pflag help formatting works correctly, but only the -c shorthand is
// intended to be user-facing.
func rejectLongCommand(rawArgs []string) error {
	for _, a := range rawArgs {
		if a == "--" {
			break // everything after "--" is a positional arg
		}
		if a == "--command" || strings.HasPrefix(a, "--command=") {
			return errors.New("unknown flag: --command")
		}
	}
	return nil
}

// executeOpts holds options for the execute function.
type executeOpts struct {
	allowedPaths             []string
	allowedCommands          []string
	allowedServices          []interp.SystemdControlGrant
	allowAllCommands         bool
	procPath                 string
	systemdTarget            interp.SystemdTargetConfig
	systemdTargetSet         bool
	mode                     interp.Mode
	disableDetailedTelemetry bool
}

func execute(ctx context.Context, script, name string, opts executeOpts, stdin io.Reader, stdout, stderr io.Writer) error {
	// Parse (also enforces the MaxScriptBytes limit).
	prog, err := interp.ParseScript(script, name)
	if err != nil {
		// Bash returns exit code 2 for syntax/parse errors.
		fmt.Fprintf(stderr, "%v\n", err)
		return interp.ExitStatus(2)
	}

	// Build runner options.
	runOpts := []interp.RunnerOption{
		interp.StdIO(stdin, stdout, stderr),
		interp.Script(script),
		interp.InvokedViaCLI(),
	}
	if len(opts.allowedPaths) > 0 {
		runOpts = append(runOpts, interp.AllowedPaths(opts.allowedPaths))
	}
	if opts.allowAllCommands {
		runOpts = append(runOpts, interpoption.AllowAllCommands().(interp.RunnerOption))
	} else if len(opts.allowedCommands) > 0 {
		runOpts = append(runOpts, interp.AllowedCommands(opts.allowedCommands))
	}
	if len(opts.allowedServices) > 0 {
		runOpts = append(runOpts, interp.AllowedSystemServices(opts.allowedServices))
	}
	if opts.procPath != "" {
		runOpts = append(runOpts, interp.ProcPath(opts.procPath))
	}
	if opts.systemdTargetSet {
		runOpts = append(runOpts, interp.WithSystemdTarget(opts.systemdTarget))
	}
	if opts.mode != "" {
		runOpts = append(runOpts, interp.WithMode(opts.mode))
	}
	if opts.disableDetailedTelemetry {
		runOpts = append(runOpts, interp.DisableDetailedTelemetry())
	}

	runner, err := interp.New(runOpts...)
	if err != nil {
		return err
	}
	defer runner.Close()

	return runner.Run(ctx, prog)
}

func parseAllowedServices(value string) []interp.SystemdControlGrant {
	if value == "" {
		return nil
	}

	entries := strings.Split(value, ",")
	grants := make([]interp.SystemdControlGrant, 0, len(entries))
	for _, entry := range entries {
		separator := strings.LastIndexByte(entry, ':')
		if separator < 0 {
			grants = append(grants, interp.SystemdControlGrant{Service: entry})
			continue
		}

		actionSpec := entry[separator+1:]
		var actionNames []string
		if actionSpec != "" {
			actionNames = strings.Split(actionSpec, "+")
		}
		actions := make([]interp.SystemServiceAction, len(actionNames))
		for i, action := range actionNames {
			actions[i] = interp.SystemServiceAction(action)
		}
		selector := entry[:separator]
		grants = append(grants, interp.SystemdControlGrant{Service: selector, Actions: actions})
	}
	return grants
}

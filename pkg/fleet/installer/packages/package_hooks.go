// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

const packageHookStderrLimit = 16 * 1024

// HasPackageHooks reports whether a package opts into package-owned lifecycle
// hooks. Only an absent hooks directory permits compiled-recipe fallback.
// Symlinks and invalid layouts are errors, including dangling symlinks.
// The package path itself may be a repository's stable/experiment symlink.
func HasPackageHooks(packagePath string) (bool, error) {
	if packagePath == "" {
		return false, errors.New("package hook path is empty")
	}
	hooksPath := filepath.Join(packagePath, "hooks")
	info, err := os.Lstat(hooksPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("could not inspect package hooks directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("package hooks must be a directory, not a symlink or other file: %s", hooksPath)
	}
	return true, nil
}

// legacyOnlyAsyncPreRemove keeps the Windows .NET garbage-collection recipe for
// hookless versions. An opted-in version owns external cleanup in its lifecycle
// hooks too; invoking the compiled cleanup would mix the two implementations.
func legacyOnlyAsyncPreRemove(legacy repository.PreRemoveHook) repository.PreRemoveHook {
	return func(ctx context.Context, packagePath string) (bool, error) {
		owned, err := HasPackageHooks(packagePath)
		if err != nil {
			return false, err
		}
		if owned {
			return true, nil
		}
		return legacy(ctx, packagePath)
	}
}

func resolvePackageHook(packagePath, event, platform string) (string, error) {
	// Do not accept a path from HookContext or introduce an event the installer
	// never dispatches (in particular, there is currently no postRemove event).
	switch event {
	case "preInstall", "postInstall", "preRemove",
		"preStartExperiment", "postStartExperiment", "preStopExperiment", "postStopExperiment",
		"prePromoteExperiment", "postPromoteExperiment", "postStartConfigExperiment",
		"preStopConfigExperiment", "postPromoteConfigExperiment", "resumeConfigExperiment", "preInstallExtension",
		"postInstallExtension", "preRemoveExtension":
	default:
		return "", fmt.Errorf("unsupported package hook event: %q", event)
	}
	suffixes := []string{""}
	if platform == "windows" {
		suffixes = []string{".exe", ".ps1", ".bat"}
	}
	for _, suffix := range suffixes {
		hookPath := filepath.Join(packagePath, "hooks", event+suffix)
		info, err := os.Lstat(hookPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("could not inspect package hook: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("package hook must be a regular file, not a symlink or other file: %s", hookPath)
		}
		if platform != "windows" && info.Mode()&0111 == 0 {
			return "", fmt.Errorf("package hook is not executable: %s", hookPath)
		}
		return filepath.Abs(hookPath)
	}
	// The directory, not each file, is the opt-in boundary. An omitted event is
	// intentionally a no-op and must never execute a compiled recipe.
	return "", nil
}

// runPackageHook returns owned=true even for an omitted or failed hook. Callers
// may use compiled recipes only if owned=false AND err=nil.
func runPackageHook(hookCtx HookContext, config *env.Env) (owned bool, err error) {
	if hookCtx.PackageType != PackageTypeOCI {
		return false, nil
	}
	owned, err = HasPackageHooks(hookCtx.PackagePath)
	if err != nil || !owned {
		return owned, err
	}
	// Do not use HookContext.StartSpan here: it includes WindowsArgs, which can
	// contain credentials. Nor use TracedCmd.Run: it buffers output without a cap.
	span, ctx := telemetry.StartSpanFromContext(hookCtx, fmt.Sprintf("package.%s.%s", hookCtx.Package, hookCtx.Hook))
	defer func() { span.Finish(err) }()
	span.SetTag("package", hookCtx.Package)
	span.SetTag("package_type", hookCtx.PackageType)
	span.SetTag("hook", hookCtx.Hook)
	span.SetTag("hook.source", "package")
	hookPath, err := resolvePackageHook(hookCtx.PackagePath, hookCtx.Hook, runtime.GOOS)
	if err != nil {
		return true, err
	}
	if hookPath == "" {
		span.SetTag("hook.omitted", true)
		return true, nil
	}
	timeout, err := config.GetPackageHookTimeout()
	if err != nil {
		return true, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	input, err := json.Marshal(hookCtx)
	if err != nil {
		return true, fmt.Errorf("could not serialize package hook context: %w", err)
	}
	cmd, err := newPackageHookCommand(ctx, hookPath)
	if err != nil {
		return true, err
	}
	cmd.Dir = hookCtx.PackagePath
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(), config.ToEnv()...)
	cmd.Env = append(cmd.Env, telemetry.EnvFromContext(ctx)...)
	// nil Stdout sends output to the null device. Even if descendants inherit
	// stderr, a completed/killed hook cannot hold the installer open indefinitely.
	cmd.WaitDelay = time.Second
	var stderr limitedHookOutput
	cmd.Stderr = &stderr
	err = cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) && cmd.Cancel != nil {
		// The main hook exited but a descendant retained its output pipe.
		_ = cmd.Cancel()
	}
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	span.SetTag("exit_code", exitCode)
	output, scrubErr := scrubber.ScrubBytes(stderr.Bytes())
	if scrubErr != nil {
		output = []byte("[could not scrub hook stderr]")
	}
	if len(output) > packageHookStderrLimit {
		output = output[:packageHookStderrLimit]
		stderr.truncated = true
	}
	span.SetTag("hook.stderr_truncated", stderr.truncated)
	if len(output) != 0 {
		span.SetTag("hook.stderr", string(output))
	}
	if ctx.Err() != nil {
		span.SetTag("hook.timed_out", errors.Is(ctx.Err(), context.DeadlineExceeded))
		err = errors.Join(ctx.Err(), err)
	}
	if err != nil {
		truncated := ""
		if stderr.truncated {
			truncated = " [stderr truncated]"
		}
		return true, fmt.Errorf("package hook %s failed%s: %w\n%s", hookCtx.Hook, truncated, err, output)
	}
	return true, nil
}

type limitedHookOutput struct {
	// Do not embed bytes.Buffer: its promoted ReadFrom method would let
	// os/exec's io.Copy bypass Write and grow the buffer without a bound.
	buffer    bytes.Buffer
	truncated bool
}

func (b *limitedHookOutput) Bytes() []byte { return b.buffer.Bytes() }

func (b *limitedHookOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := packageHookStderrLimit - b.buffer.Len()
	if len(p) > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
)

func TestHasPackageHooks(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		owned, err := HasPackageHooks(t.TempDir())
		require.NoError(t, err)
		assert.False(t, owned)
	})
	t.Run("empty directory opts in", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "hooks"), 0755))
		owned, err := HasPackageHooks(root)
		require.NoError(t, err)
		assert.True(t, owned)
	})
	t.Run("non-directory fails closed", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "hooks"), nil, 0644))
		_, err := HasPackageHooks(root)
		require.Error(t, err)
	})
	for _, dangling := range []bool{false, true} {
		name := "symlink"
		if dangling {
			name = "dangling symlink"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			target := t.TempDir()
			if dangling {
				target = filepath.Join(target, "missing")
			}
			require.NoError(t, os.Symlink(target, filepath.Join(root, "hooks")))
			_, err := HasPackageHooks(root)
			require.Error(t, err)
		})
	}
	t.Run("repository symlink is supported", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "v1")
		require.NoError(t, os.MkdirAll(filepath.Join(target, "hooks"), 0755))
		stable := filepath.Join(root, "stable")
		require.NoError(t, os.Symlink(target, stable))
		owned, err := HasPackageHooks(stable)
		require.NoError(t, err)
		assert.True(t, owned)
	})
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		t.Run("inaccessible directory is not absence", func(t *testing.T) {
			root := t.TempDir()
			hooks := filepath.Join(root, "hooks")
			require.NoError(t, os.Mkdir(hooks, 0000))
			t.Cleanup(func() { _ = os.Chmod(hooks, 0755) })
			ctx := HookContext{Context: context.Background(), PackageType: PackageTypeOCI, PackagePath: root, Hook: "postInstall"}
			owned, err := runPackageHook(ctx, &env.Env{})
			require.Error(t, err)
			assert.True(t, owned)
		})
	}
}

func TestResolvePackageHook(t *testing.T) {
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			root := t.TempDir()
			hooks := filepath.Join(root, "hooks")
			require.NoError(t, os.Mkdir(hooks, 0755))
			suffixes := []string{""}
			if platform == "windows" {
				suffixes = []string{".exe", ".ps1", ".bat"}
			}
			for _, suffix := range append([]string{".sh", ".cmd"}, suffixes...) {
				require.NoError(t, os.WriteFile(filepath.Join(hooks, "postInstall"+suffix), nil, 0755))
			}
			for _, suffix := range suffixes {
				hook, err := resolvePackageHook(root, "postInstall", platform)
				require.NoError(t, err)
				assert.Equal(t, filepath.Join(hooks, "postInstall"+suffix), hook)
				require.NoError(t, os.Remove(hook))
			}
			hook, err := resolvePackageHook(root, "postInstall", platform)
			require.NoError(t, err)
			assert.Empty(t, hook, "other extensions do not opt a missing event back into compiled recipes")
		})
	}
	for _, event := range []string{"../outside", "/outside", `..\outside`, "postRemove", "unknown"} {
		t.Run(event, func(t *testing.T) {
			_, err := resolvePackageHook(t.TempDir(), event, "linux")
			require.Error(t, err)
		})
	}
	for _, kind := range []string{"directory", "symlink", "not executable"} {
		t.Run(kind, func(t *testing.T) {
			if runtime.GOOS == "windows" && kind == "not executable" {
				t.Skip("Windows does not have Unix executable permission bits")
			}
			root := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(root, "hooks"), 0755))
			hook := filepath.Join(root, "hooks", "postInstall")
			switch kind {
			case "directory":
				require.NoError(t, os.Mkdir(hook, 0755))
			case "symlink":
				require.NoError(t, os.Symlink(filepath.Join(root, "missing"), hook))
			default:
				require.NoError(t, os.WriteFile(hook, nil, 0644))
			}
			_, err := resolvePackageHook(root, "postInstall", "linux")
			require.Error(t, err)
		})
	}
}

func TestLegacyOnlyAsyncPreRemove(t *testing.T) {
	for _, layout := range []string{"legacy", "package-owned", "invalid"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			switch layout {
			case "package-owned":
				require.NoError(t, os.Mkdir(filepath.Join(root, "hooks"), 0755))
			case "invalid":
				require.NoError(t, os.WriteFile(filepath.Join(root, "hooks"), nil, 0644))
			}
			called := false
			legacyErr := errors.New("legacy cleanup still pending")
			callback := legacyOnlyAsyncPreRemove(func(ctx context.Context, path string) (bool, error) {
				called = true
				assert.Equal(t, root, path)
				assert.NotNil(t, ctx)
				return false, legacyErr
			})
			remove, err := callback(context.Background(), root)
			switch layout {
			case "legacy":
				assert.True(t, called)
				assert.False(t, remove)
				require.ErrorIs(t, err, legacyErr)
			case "package-owned":
				assert.False(t, called)
				assert.True(t, remove)
				require.NoError(t, err)
			case "invalid":
				assert.False(t, called)
				assert.False(t, remove)
				require.Error(t, err)
			}
		})
	}
}

func TestPackageHookNonOCITypesStayLegacy(t *testing.T) {
	root := t.TempDir()
	// A malformed hooks layout must not affect unrelated package formats.
	require.NoError(t, os.WriteFile(filepath.Join(root, "hooks"), nil, 0644))
	for _, packageType := range []PackageType{PackageTypeDEB, PackageTypeRPM, PackageTypeMSI, PackageTypeDMG, ""} {
		ctx := HookContext{Context: context.Background(), PackageType: packageType, PackagePath: root, Hook: "postInstall"}
		owned, err := runPackageHook(ctx, &env.Env{})
		require.NoError(t, err)
		assert.False(t, owned)
	}
}

func TestPackageHookMissingEventsAreNoOps(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "hooks"), 0755))
	for _, event := range []string{
		"preInstall", "postInstall", "preRemove", "preStartExperiment", "postStartExperiment",
		"preStopExperiment", "postStopExperiment", "prePromoteExperiment", "postPromoteExperiment",
		"postStartConfigExperiment", "preStopConfigExperiment", "postPromoteConfigExperiment",
		"resumeConfigExperiment",
		"preInstallExtension", "postInstallExtension", "preRemoveExtension",
	} {
		ctx := HookContext{Context: context.Background(), Package: agentPackage, PackageType: PackageTypeOCI, PackagePath: root, Hook: event}
		owned, err := runPackageHook(ctx, &env.Env{})
		require.NoError(t, err)
		assert.True(t, owned, event)
	}
}

func writeUnixPackageHook(t *testing.T, body string) HookContext {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixture")
	}
	root := filepath.Join(t.TempDir(), "package with spaces")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "hooks"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "hooks", "postInstall"), []byte("#!/bin/sh\n"+body), 0755))
	return HookContext{
		Context: context.Background(), Package: "fixture", PackageType: PackageTypeOCI,
		PackagePath: root, Hook: "postInstall", Upgrade: true, WindowsArgs: []string{"example=value"}, Extension: "fixture-extension",
	}
}

func TestPackageHookJSONAndWorkingDirectory(t *testing.T) {
	ctx := writeUnixPackageHook(t, "cat > input.json\npwd > cwd\nprintf '%s' \"$DD_SITE\" > environment\n")
	owned, err := runPackageHook(ctx, &env.Env{Site: "example.test"})
	require.NoError(t, err)
	require.True(t, owned)
	input, err := os.ReadFile(filepath.Join(ctx.PackagePath, "input.json"))
	require.NoError(t, err)
	expected, err := json.Marshal(ctx)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(input))
	cwd, err := os.ReadFile(filepath.Join(ctx.PackagePath, "cwd"))
	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(ctx.PackagePath)
	require.NoError(t, err)
	assert.Equal(t, resolved, strings.TrimSpace(string(cwd)))
	gotEnv, err := os.ReadFile(filepath.Join(ctx.PackagePath, "environment"))
	require.NoError(t, err)
	assert.Equal(t, "example.test", string(gotEnv))
}

func TestPackageHookFailureAndBoundedOutput(t *testing.T) {
	ctx := writeUnixPackageHook(t, "printf 'not captured'\nprintf 'api_key: "+strings.Repeat("a", 32)+"\\n"+strings.Repeat("x", 32*1024)+"' >&2\nexit 42\n")
	owned, err := runPackageHook(ctx, &env.Env{})
	require.Error(t, err)
	assert.True(t, owned)
	var exitErr *exec.ExitError
	require.True(t, errors.As(err, &exitErr))
	assert.Equal(t, 42, exitErr.ExitCode())
	assert.NotContains(t, err.Error(), "not captured")
	assert.NotContains(t, err.Error(), strings.Repeat("a", 32))
	assert.Contains(t, err.Error(), "truncated")
	assert.Less(t, len(err.Error()), 17*1024)
}

func TestPackageHookCancellationAndInvalidTimeout(t *testing.T) {
	t.Run("canceled before start", func(t *testing.T) {
		ctx := writeUnixPackageHook(t, "touch must-not-run\n")
		canceled, cancel := context.WithCancel(ctx.Context)
		cancel()
		ctx.Context = canceled
		owned, err := runPackageHook(ctx, &env.Env{})
		require.ErrorIs(t, err, context.Canceled)
		assert.True(t, owned)
		assert.NoFileExists(t, filepath.Join(ctx.PackagePath, "must-not-run"))
	})
	t.Run("invalid timeout cannot disable limit", func(t *testing.T) {
		ctx := writeUnixPackageHook(t, "touch must-not-run\n")
		owned, err := runPackageHook(ctx, &env.Env{PackageHookTimeout: "0"})
		require.Error(t, err)
		assert.True(t, owned)
		assert.NoFileExists(t, filepath.Join(ctx.PackagePath, "must-not-run"))
	})
}

func TestPackageHookBypassesBundledInstaller(t *testing.T) {
	root := t.TempDir()
	repos := repository.NewRepositories(root, nil)
	stable := repos.Get(agentPackage).StablePath()
	require.NoError(t, os.MkdirAll(filepath.Join(stable, "hooks"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(stable, "embedded", "bin"), 0755))
	// A malformed bundled installer would fail if hooksCLI delegated to it.
	require.NoError(t, os.WriteFile(filepath.Join(stable, "embedded", "bin", "installer"), []byte("not an executable"), 0755))
	hooks := NewHooks(&env.Env{}, repos)
	require.NoError(t, hooks.PostInstall(context.Background(), agentPackage, PackageTypeOCI, false, nil))
}

func TestPreInstallUsesIncomingPackageHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixture")
	}
	root := t.TempDir()
	repos := repository.NewRepositories(root, nil)
	staged := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(staged, "hooks"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(staged, "hooks", "preInstall"), []byte("#!/bin/sh\ncat > input.json\n"), 0755))
	hooks := NewHooks(&env.Env{}, repos)
	require.NoError(t, hooks.PreInstall(context.Background(), agentPackage, PackageTypeOCI, false, staged))
	input, err := os.ReadFile(filepath.Join(staged, "input.json"))
	require.NoError(t, err)
	var actual HookContext
	require.NoError(t, json.Unmarshal(input, &actual))
	assert.Equal(t, staged, actual.PackagePath)
	assert.Equal(t, "preInstall", actual.Hook)
	assert.NoDirExists(t, repos.Get(agentPackage).StablePath())
}

func TestResumeConfigExperimentUsesPackageHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixture")
	}
	repos := repository.NewRepositories(t.TempDir(), nil)
	stable := repos.Get(agentPackage).StablePath()
	require.NoError(t, os.MkdirAll(filepath.Join(stable, "hooks"), 0755))
	hooks := NewHooks(&env.Env{}, repos)
	// An omitted recovery event is also owned by the package, not the compiled recipe.
	require.NoError(t, hooks.ResumeConfigExperiment(context.Background(), agentPackage))
	require.NoError(t, os.WriteFile(filepath.Join(stable, "hooks", "resumeConfigExperiment"), []byte("#!/bin/sh\ncat > resume-input.json\n"), 0755))
	require.NoError(t, hooks.ResumeConfigExperiment(context.Background(), agentPackage))
	input, err := os.ReadFile(filepath.Join(stable, "resume-input.json"))
	require.NoError(t, err)
	var actual HookContext
	require.NoError(t, json.Unmarshal(input, &actual))
	assert.Equal(t, stable, actual.PackagePath)
	assert.Equal(t, agentPackage, actual.Package)
	assert.Equal(t, "resumeConfigExperiment", actual.Hook)
}

func TestLimitedHookOutput(t *testing.T) {
	output := &limitedHookOutput{}
	input := []byte(strings.Repeat("x", packageHookStderrLimit+123))
	n, err := output.Write(input)
	require.NoError(t, err)
	assert.Equal(t, len(input), n, "discarded output must still be reported as consumed")
	assert.Len(t, output.Bytes(), packageHookStderrLimit)
	assert.True(t, output.truncated)

	// os/exec copies stderr from a pipe. Exercise io.Copy without a source
	// WriterTo so an accidentally promoted bytes.Buffer.ReadFrom is caught.
	output = &limitedHookOutput{}
	written, err := io.Copy(output, struct{ io.Reader }{strings.NewReader(string(input))})
	require.NoError(t, err)
	assert.Equal(t, int64(len(input)), written)
	assert.Len(t, output.Bytes(), packageHookStderrLimit)
	assert.True(t, output.truncated)
}

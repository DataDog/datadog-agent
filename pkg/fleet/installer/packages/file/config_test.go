// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package file

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

const scriptConfigPath = "private-action-runner/script-config.yaml"

func configExample(t *testing.T, content string) (string, string) {
	t.Helper()
	root := t.TempDir()
	live := filepath.Join(root, scriptConfigPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(live), 0755))
	require.NoError(t, os.WriteFile(live+".example", []byte(content), 0644))
	return root, live
}

func TestEnsureConfigFromExampleCreatesRestrictiveDefault(t *testing.T) {
	root, live := configExample(t, "packaged default\n")
	require.NoError(t, EnsureConfigFromExample(root, scriptConfigPath))
	content, err := os.ReadFile(live)
	require.NoError(t, err)
	require.Equal(t, "packaged default\n", string(content))
	info, err := os.Stat(live)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0400), info.Mode().Perm())
	entries, err := os.ReadDir(filepath.Dir(live))
	require.NoError(t, err)
	require.Len(t, entries, 2, "temporary files must be removed")
}

func TestEnsureConfigFromExamplePreservesExistingFile(t *testing.T) {
	for _, content := range []string{"customer scripts and credentials\n", ""} {
		t.Run(content, func(t *testing.T) {
			root, live := configExample(t, "packaged default\n")
			require.NoError(t, os.WriteFile(live, []byte(content), 0400))
			before, err := os.Stat(live)
			require.NoError(t, err)

			require.NoError(t, EnsureConfigFromExample(root, scriptConfigPath))
			after, err := os.Stat(live)
			require.NoError(t, err)
			require.True(t, os.SameFile(before, after))
			require.Equal(t, before.Mode(), after.Mode())
			require.Equal(t, before.ModTime(), after.ModTime())
			actual, err := os.ReadFile(live)
			require.NoError(t, err)
			require.Equal(t, content, string(actual))
		})
	}
}

func TestEnsureConfigFromExampleMissingExample(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, EnsureConfigFromExample(root, scriptConfigPath))
	require.NoFileExists(t, filepath.Join(root, scriptConfigPath))
	require.NoError(t, EnsureConfigFromExample(filepath.Join(root, "absent"), scriptConfigPath))
}

func TestEnsureConfigFromExamplePreservesSymlink(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "dangling", true: "existing"}[exists], func(t *testing.T) {
			root, live := configExample(t, "packaged default\n")
			target := filepath.Join(t.TempDir(), "customer.yaml")
			if exists {
				require.NoError(t, os.WriteFile(target, []byte("customer config\n"), 0400))
			}
			require.NoError(t, os.Symlink(target, live))

			require.NoError(t, EnsureConfigFromExample(root, scriptConfigPath))
			actual, err := os.Readlink(live)
			require.NoError(t, err)
			require.Equal(t, target, actual)
			if exists {
				content, err := os.ReadFile(target)
				require.NoError(t, err)
				require.Equal(t, "customer config\n", string(content))
			} else {
				require.NoFileExists(t, target)
			}
		})
	}
}

func TestEnsureConfigFromExampleRejectsNonRegularExample(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			root, live := configExample(t, "packaged default\n")
			require.NoError(t, os.Remove(live+".example"))
			switch kind {
			case "directory":
				require.NoError(t, os.Mkdir(live+".example", 0700))
			case "symlink":
				require.NoError(t, os.Symlink("/etc/passwd", live+".example"))
			case "fifo":
				require.NoError(t, syscall.Mkfifo(live+".example", 0600))
			}
			require.ErrorContains(t, EnsureConfigFromExample(root, scriptConfigPath), "not a regular file")
			require.NoFileExists(t, live)
		})
	}
}

func TestEnsureConfigFromExampleRejectsEscapingParent(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "script-config.yaml.example"), []byte("outside root"), 0400))
	require.NoError(t, os.Symlink(target, filepath.Join(root, "private-action-runner")))

	require.Error(t, EnsureConfigFromExample(root, scriptConfigPath))
	require.NoFileExists(t, filepath.Join(target, "script-config.yaml"))
}

func TestEnsureConfigFromExampleWriteFailureLeavesNoLiveFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root, live := configExample(t, "packaged default\n")
	dir := filepath.Dir(live)
	require.NoError(t, os.Chmod(dir, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0700)) })

	require.ErrorIs(t, EnsureConfigFromExample(root, scriptConfigPath), os.ErrPermission)
	require.NoFileExists(t, live)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "failed creation must not leave temporary files")
}

func TestEnsureConfigFromExampleConcurrentCustomerCreation(t *testing.T) {
	root, live := configExample(t, strings.Repeat("packaged default\n", 4096))
	result := make(chan error, 1)
	go func() { result <- EnsureConfigFromExample(root, scriptConfigPath) }()

	customer, err := os.OpenFile(live, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0400)
	if err == nil {
		_, err = customer.WriteString("customer config\n")
		require.NoError(t, err)
		require.NoError(t, customer.Close())
	} else {
		require.ErrorIs(t, err, os.ErrExist)
	}
	require.NoError(t, <-result)
	if customer != nil {
		content, err := os.ReadFile(live)
		require.NoError(t, err)
		require.Equal(t, "customer config\n", string(content))
	}
}

func TestEnsureConfigFromExampleConcurrentInitialization(t *testing.T) {
	content := strings.Repeat("packaged default\n", 4096)
	root, live := configExample(t, content)
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 16)
	for range cap(errs) {
		wg.Go(func() {
			<-start
			errs <- EnsureConfigFromExample(root, scriptConfigPath)
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	actual, err := os.ReadFile(live)
	require.NoError(t, err)
	require.Equal(t, content, string(actual))
	entries, err := os.ReadDir(filepath.Dir(live))
	require.NoError(t, err)
	require.Len(t, entries, 2)
}

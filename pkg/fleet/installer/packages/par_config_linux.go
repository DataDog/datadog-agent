// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

const parMigrationDirectory = "/etc/datadog-agent-par-config-migration"
const parConfigRelativePath = "private-action-runner/script-config.yaml"

// The native pre-install scripts retain the original inode in a root-only directory.
// Keeping it on the config filesystem preserves ownership, ACLs and SELinux labels.
type parConfigMigration struct {
	state      *os.Root
	configPath string
}

func openPARConfigMigration(statePath, configPath string) (*parConfigMigration, error) {
	info, err := os.Lstat(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || info.Sys().(*syscall.Stat_t).Uid != 0 {
		return nil, fmt.Errorf("PAR migration directory must be a root-owned 0700 directory")
	}
	state, err := os.OpenRoot(statePath)
	if err != nil {
		return nil, err
	}
	info, err = state.Lstat("script-config.yaml")
	if errors.Is(err, os.ErrNotExist) {
		state.Close()
		return nil, nil
	}
	if err != nil {
		state.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		state.Close()
		return nil, fmt.Errorf("unsupported PAR migration snapshot type")
	}
	return &parConfigMigration{state: state, configPath: configPath}, nil
}

func (m *parConfigMigration) close() {
	if m != nil {
		m.state.Close()
	}
}

// Detach a restored live entry before filesystem setup can change the retained inode.
func (m *parConfigMigration) prepare() error {
	root, err := os.OpenRoot(m.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	live, err := root.Lstat(parConfigRelativePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	saved, err := m.state.Lstat("script-config.yaml")
	if err != nil {
		return err
	}
	if !os.SameFile(live, saved) {
		return fmt.Errorf("PAR config changed while migration is pending; retained snapshot requires manual recovery")
	}
	return root.Remove(parConfigRelativePath)
}

func (m *parConfigMigration) restore() error {
	root, err := os.OpenRoot(m.configPath)
	if err != nil {
		return err
	}
	defer root.Close()
	dest, err := root.Open(filepath.Dir(parConfigRelativePath))
	if err != nil {
		return err
	}
	defer dest.Close()
	source, err := m.state.Open(".")
	if err != nil {
		return err
	}
	defer source.Close()
	name := ".script-config-migration-" + rand.Text()
	// Directory descriptors prevent destination symlinks from redirecting privileged writes.
	if err := unix.Linkat(int(source.Fd()), "script-config.yaml", int(dest.Fd()), name, 0); err != nil {
		return fmt.Errorf("failed to link retained PAR config: %w", err)
	}
	defer unix.Unlinkat(int(dest.Fd()), name, 0)
	if err := unix.Renameat(int(dest.Fd()), name, int(dest.Fd()), "script-config.yaml"); err != nil {
		return err
	}
	return dest.Sync()
}

func (m *parConfigMigration) commit() error {
	return m.state.Remove("script-config.yaml")
}

func nativePARConfigMigration(ctx HookContext) (*parConfigMigration, error) {
	if ctx.PackageType != PackageTypeDEB && ctx.PackageType != PackageTypeRPM {
		return nil, nil
	}
	return openPARConfigMigration(parMigrationDirectory, "/etc/datadog-agent")
}

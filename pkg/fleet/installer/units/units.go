// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package units defines the service unit entries stored by the per-platform registries
// (linuxunits, darwinunits, windowsunits) and the primitives writing their files.
//
// A registry only stores units: which unit is started, enabled or gated is decided by its callers.
// Each platform only imports its own registry, so a binary embeds the files of its platform only.
package units

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

// Manager is the service manager a unit belongs to.
type Manager string

const (
	Systemd    Manager = "systemd"
	Procmgr    Manager = "procmgr"
	Upstart    Manager = "upstart"
	Sysvinit   Manager = "sysvinit"
	Launchd    Manager = "launchd"
	LaunchdGUI Manager = "launchd-gui"
	SCM        Manager = "scm"
)

// Unit is a registry entry.
type Unit struct {
	// Name is the installed name of the stable unit.
	Name    string
	Manager Manager
	// Template is the generator template rendering the unit. Empty when the installer does not
	// write the unit itself (scripts shipped by the OS package, SCM services created by the MSI...).
	Template string
	// Variant distinguishes two contents installed under the same Manager and Name.
	Variant string
	// Binary is the path, relative to the install root, of the executable the unit runs.
	Binary string
}

// ExpName returns the installed name of the experiment unit.
func (u Unit) ExpName() string {
	ext := path.Ext(u.Name)
	return strings.TrimSuffix(u.Name, ext) + "-exp" + ext
}

// InstalledName returns the installed name of the stable or experiment unit.
func (u Unit) InstalledName(exp bool) string {
	if exp {
		return u.ExpName()
	}
	return u.Name
}

// StableAndExpNames returns the installed names of the stable and experiment units.
func StableAndExpNames(units ...Unit) []string {
	return append(Names(units, false), Names(units, true)...)
}

// Names returns the installed names of units.
func Names(units []Unit, exp bool) []string {
	names := make([]string, 0, len(units))
	for _, u := range units {
		names = append(names, u.InstalledName(exp))
	}
	return names
}

// Fields returns the units held by the fields of the given registry tables, in declaration order.
func Fields(tables ...any) []Unit {
	var all []Unit
	for _, table := range tables {
		v := reflect.ValueOf(table)
		for i := range v.NumField() {
			all = append(all, v.Field(i).Interface().(Unit))
		}
	}
	return all
}

// WriteFile atomically writes name under dir, creating dir if needed.
func WriteFile(dir, name string, content []byte) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	// The temporary name ends with neither .yaml nor a unit suffix, so no manager loads it.
	tmp := "." + name + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Join(dir, tmp), err)
	}
	_, err = f.Write(content)
	if err == nil {
		err = f.Chmod(0644)
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		err = root.Rename(tmp, name)
	}
	if err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("failed to write %s: %w", filepath.Join(dir, name), err)
	}
	return nil
}

// RemoveFiles removes names from dir. Missing files are ignored, and a dir that is not a real
// directory is left alone so a symlink is never followed.
func RemoveFiles(dir string, names ...string) error {
	root, err := openRealDir(dir)
	if root == nil {
		return err
	}
	defer root.Close()
	var errs []error
	for _, name := range names {
		if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", filepath.Join(dir, name), err))
		}
	}
	return errors.Join(errs...)
}

// RemoveDirIfEmpty removes dir when it is a real, empty directory.
func RemoveDirIfEmpty(dir string) error {
	root, err := openRealDir(dir)
	if root == nil {
		return err
	}
	entries, err := readFirstEntry(root)
	root.Close()
	if err != nil || len(entries) > 0 {
		return err
	}
	if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove %s: %w", dir, err)
	}
	return nil
}

func openRealDir(dir string) (*os.Root, error) {
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return nil, nil
	}
	return os.OpenRoot(dir)
}

func readFirstEntry(root *os.Root) ([]fs.DirEntry, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return entries, err
}

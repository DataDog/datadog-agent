// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

const (
	testModuleName    = "cifs"
	testModulePathFmt = "/lib/modules/%s/kernel/fs/cifs/cifs.ko"
)

func loadModule(name string) ([]byte, error) {
	e := exec.Command("modprobe", name)
	return e.Output()
}

func compressModule(modulePath string, t *testing.T) {
	e := exec.Command("xz", "-z", modulePath)
	if err := e.Run(); err != nil {
		t.Errorf("failed to re-compress module: %v", err)
	}
}

func uncompressModule(xzModulePath string) error {
	e := exec.Command("xz", "-d", xzModulePath)
	return e.Run()
}

func getModulePath(modulePathFmt string, t *testing.T) (string, bool) {
	var wasCompressed bool
	var buf unix.Utsname
	if err := unix.Uname(&buf); err != nil {
		t.Skipf("uname failed: %v", err)
	}
	release, err := model.UnmarshalString(buf.Release[:], 65)
	if err != nil {
		t.Skipf("couldn't parse uname release: %v", err)
	}

	modulePath := fmt.Sprintf(modulePathFmt, release)
	_, err = os.Stat(modulePath)
	if err != nil {
		// check if a compressed version is present
		xzModulePath := modulePath + ".xz"
		_, err = os.Stat(xzModulePath)
		if err != nil {
			// we can't find the module, skip the test
			t.Skipf("kernel module not found, skipping: %v", err)
		}
		// uncompress module
		if err = uncompressModule(xzModulePath); err != nil {
			t.Skipf("failed to uncompress module: %v", err)
		}
		// check if the module is now ok
		_, err = os.Stat(modulePath)
		if err != nil {
			// we can't find the module, skip the test
			t.Skipf("kernel module not found, skipping: %v", err)
		}
		wasCompressed = true
	}

	// check if one of the parents of the file is a symlink
	var segment string
	var done bool
	var split []string

	for !done {
		split = append([]string{"/"}, strings.Split(modulePath, "/")...)
		for i := len(split); i > 0; i-- {
			segment, err = os.Readlink(filepath.Join(split[0:i]...))
			if err == nil {
				modulePath = filepath.Join("/", filepath.Join(segment, filepath.Join(split[i:]...)))
				break
			} else if i == 1 {
				done = true
				break
			}
		}
	}

	return modulePath, wasCompressed
}

func TestLoadModule(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping kernel module test in docker")
	}

	// before trying to load the module, some dependant modules might be needed, use modprobe to load them first
	if _, err := loadModule(testModuleName); err != nil {
		t.Skipf("failed to load %s module: %v", testModuleName, err)
	}

	modulePath, wasCompressed := getModulePath(testModulePathFmt, t)
	if wasCompressed {
		// we need to re-compress the module, otherwise it breaks modprobe
		defer compressModule(modulePath, t)
	}

	// make sure the xfs module isn't currently loaded
	err := unix.DeleteModule(testModuleName, unix.O_NONBLOCK)
	if err != nil {
		t.Skipf("couldn't delete %s module: %v", testModuleName, err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_load_module_from_memory",
			Expression: fmt.Sprintf(`load_module.name == "%s" && load_module.loaded_from_memory == true`, testModuleName),
		},
		{
			ID:         "test_load_module",
			Expression: fmt.Sprintf(`load_module.name == "%s" && load_module.file.path == "%s" && load_module.loaded_from_memory == false`, testModuleName, modulePath),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("init_module", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var f *os.File
			f, err = os.Open(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't open module: %w", err)
			}
			defer f.Close()

			var module []byte
			module, err = io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("couldn't load module content: %w", err)
			}

			if err = unix.InitModule(module, ""); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, unix.O_NONBLOCK)
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "test_load_module_from_memory", r.ID, "invalid rule triggered")
			assert.Equal(t, "", event.ResolveFilePath(&event.LoadModule.File), "shouldn't get a path")

			if !validateLoadModuleNoFileSchema(t, event) {
				t.Error(event.String())
			}
		})
	})

	t.Run("finit_module", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var f *os.File
			f, err = os.Open(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't open module: %w", err)
			}
			defer f.Close()

			if err = unix.FinitModule(int(f.Fd()), "", 0); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, unix.O_NONBLOCK)
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "test_load_module", r.ID, "invalid rule triggered")

			if !validateLoadModuleSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}

func TestUnloadModule(t *testing.T) {
	if testEnvironment == DockerEnvironment {
		t.Skip("skipping kernel module test in docker")
	}

	// before trying to load the module, some dependant modules might be needed, use modprobe to load them first
	if _, err := loadModule(testModuleName); err != nil {
		t.Skipf("failed to load %s module: %v", testModuleName, err)
	}

	modulePath, wasCompressed := getModulePath(testModulePathFmt, t)
	if wasCompressed {
		// we need to re-compress the module, otherwise it breaks modprobe
		defer compressModule(modulePath, t)
	}

	// make sure the xfs module isn't currently loaded
	err := unix.DeleteModule(testModuleName, unix.O_NONBLOCK)
	if err != nil {
		t.Skipf("couldn't delete %s module: %v", testModuleName, err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_unload_module",
			Expression: fmt.Sprintf(`unload_module.name == "%s"`, testModuleName),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("delete_module", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			var f *os.File
			f, err = os.Open(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't open module: %w", err)
			}
			defer f.Close()

			var module []byte
			module, err = io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("couldn't load module content: %w", err)
			}

			if err = unix.InitModule(module, ""); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, unix.O_NONBLOCK)
		}, func(event *sprobe.Event, r *rules.Rule) {
			assert.Equal(t, "test_unload_module", r.ID, "invalid rule triggered")

			if !validateUnloadModuleSchema(t, event) {
				t.Error(event.String())
			}
		})
	})
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
	release, err := kernel.Release()
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

func TestKworker(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("skipping kernel module test in docker")
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_load_module_kworker",
			Expression: `load_module.name == "xt_LED" && process.is_kworker`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	_ = unix.DeleteModule("xt_LED", 0)

	cmd := exec.Command("modprobe", "xt_LED")
	if err := cmd.Run(); err != nil {
		t.Skip("required kernel module not available")
	}

	defer func() {
		cmd := exec.Command("iptables", "-D", "INPUT", "-p", "tcp", "--dport", "2222", "-j", "LED", "--led-trigger-id", "123")
		if err := cmd.Run(); err != nil {
			t.Error(err)
		}

		if err := retry.Do(func() error { return unix.DeleteModule("xt_LED", 0) }); err != nil {
			t.Error(err)
		}
	}()

	test.WaitSignal(t, func() error {
		if err := unix.DeleteModule("xt_LED", 0); err != nil {
			return err
		}

		cmd := exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", "2222", "-j", "LED", "--led-trigger-id", "123")
		if err := cmd.Run(); err != nil {
			return err
		}

		return nil
	}, func(event *model.Event, r *rules.Rule) {
		assert.Equal(t, "test_load_module_kworker", r.ID, "invalid rule triggered")
	})
}

func TestLoadModule(t *testing.T) {
	SkipIfNotAvailable(t)

	if os.Getenv("CI") == "true" {
		t.Skip("TestLoadModule is known to be flaky")
	}

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
	err := unix.DeleteModule(testModuleName, 0)
	if err != nil {
		t.Skipf("couldn't delete %s module: %v", testModuleName, err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_load_module_from_memory",
			Expression: fmt.Sprintf(`load_module.name == "%s" && load_module.loaded_from_memory == true && load_module.args == "" && !process.is_kworker`, testModuleName),
		},
		{
			ID:         "test_load_module",
			Expression: fmt.Sprintf(`load_module.name == "%s" && load_module.file.path == "%s" && load_module.loaded_from_memory == false && load_module.args == "" && !process.is_kworker`, testModuleName, modulePath),
		},
		{
			ID:         "test_load_module_with_params",
			Expression: fmt.Sprintf(`load_module.name == "%s" && load_module.argv in ["cifs_max_pending=2"]`, testModuleName),
		},
		{
			ID:         "test_load_module_with_truncated_params",
			Expression: fmt.Sprintf(`load_module.name == "%s" && load_module.argv in [r"CIFSMaxBufSize=.*"]`, testModuleName),
		},
		{
			ID:         "test_load_module_with_params_from_memory",
			Expression: fmt.Sprintf(`load_module.name == "%s" && load_module.argv in ["cifs_min_rcv=6"] && load_module.loaded_from_memory == true`, testModuleName),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("init_module", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			module, err := os.ReadFile(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't load module content: %w", err)
			}

			if err = unix.InitModule(module, ""); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, 0)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "test_load_module_from_memory", r.ID, "invalid rule triggered")

			value, _ := event.GetFieldValue("event.async")
			assert.Equal(t, value.(bool), false)

			event.ResolveFields()
			assert.Equal(t, "", event.LoadModule.File.PathnameStr, "shouldn't get a path")

			assert.Empty(t, event.LoadModule.Argv, "shouldn't get args")

			test.validateLoadModuleNoFileSchema(t, event)
		})
	})

	t.Run("finit_module", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			f, err := os.Open(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't open module: %w", err)
			}
			defer f.Close()

			if err = unix.FinitModule(int(f.Fd()), "", 0); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, 0)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "test_load_module", r.ID, "invalid rule triggered")

			test.validateLoadModuleSchema(t, event)
		})
	})

	t.Run("load_module_with_params", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			f, err := os.Open(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't open module: %w", err)
			}
			defer f.Close()

			if err = unix.FinitModule(int(f.Fd()), "cifs_max_pending=2 cifs_min_small=2", 0); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}
			return unix.DeleteModule(testModuleName, 0)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "test_load_module_with_params", r.ID, "wrong rule triggered")
			assertFieldEqual(t, event, "load_module.args", "cifs_max_pending=2 cifs_min_small=2")
			assertFieldEqual(t, event, "load_module.loaded_from_memory", false)
			assertFieldEqual(t, event, "load_module.args_truncated", false)
			test.validateLoadModuleSchema(t, event)
		})
	})

	t.Run("load_module_with_params_from_memory", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			module, err := os.ReadFile(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't load module content: %w", err)
			}

			if err = unix.InitModule(module, "cifs_min_rcv=6"); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, 0)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "test_load_module_with_params_from_memory", r.ID, "wrong rule triggered")
			assertFieldEqual(t, event, "load_module.argv", []string{"cifs_min_rcv=6"})
			assertFieldEqual(t, event, "load_module.loaded_from_memory", true)
			assertFieldEqual(t, event, "load_module.args_truncated", false)
			test.validateLoadModuleSchema(t, event)
		})
	})

	t.Run("load_module_with_truncated_params", func(t *testing.T) {
		var args []string
		for i := 0; i != 10; i++ {
			args = append(args, fmt.Sprintf("CIFSMaxBufSize=%d", 8192+i))
		}

		test.WaitSignal(t, func() error {
			module, err := os.ReadFile(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't load module content: %w", err)
			}

			if err = unix.InitModule(module, strings.Join(args, " ")); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, 0)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "test_load_module_with_truncated_params", r.ID, "wrong rule triggered")
			assertFieldEqual(t, event, "load_module.argv", args[0:6])
			assertFieldEqual(t, event, "load_module.args", strings.Join(args[0:6], " "))
			assertFieldEqual(t, event, "load_module.loaded_from_memory", true)
			assertFieldEqual(t, event, "load_module.args_truncated", true)
			test.validateLoadModuleSchema(t, event)
		})
	})
}

func TestUnloadModule(t *testing.T) {
	SkipIfNotAvailable(t)

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
	err := unix.DeleteModule(testModuleName, 0)
	if err != nil {
		t.Skipf("couldn't delete %s module: %v", testModuleName, err)
	}

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_unload_module",
			Expression: fmt.Sprintf(`unload_module.name == "%s"`, testModuleName),
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("delete_module", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			module, err := os.ReadFile(modulePath)
			if err != nil {
				return fmt.Errorf("couldn't load module content: %w", err)
			}

			if err = unix.InitModule(module, ""); err != nil {
				return fmt.Errorf("couldn't insert module: %w", err)
			}

			return unix.DeleteModule(testModuleName, 0)
		}, func(event *model.Event, r *rules.Rule) {
			assert.Equal(t, "test_unload_module", r.ID, "invalid rule triggered")

			test.validateUnloadModuleSchema(t, event)
		})
	})
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packagecore

import (
	"fmt"
	"os"
	"path/filepath"
)

var coreChecks = []string{"cpu", "memory", "disk", "network", "uptime", "load", "io", "file_handle"}

const coreCheckConfig = "init_config:\ninstances:\n  - {}\n"

// PrepareChecks never adopts an existing binary installer's integrations. Reuse
// requires exactly this fixed inventory, including file types and contents.
func PrepareChecks(dir string) error {
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		tmp, err := os.MkdirTemp(filepath.Dir(dir), ".package-core-checks-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		for _, name := range coreChecks {
			folder := filepath.Join(tmp, name+".d")
			if err = os.Mkdir(folder, 0755); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(folder, "conf.yaml"), []byte(coreCheckConfig), 0600); err != nil {
				return err
			}
		}
		if err = os.Rename(tmp, dir); err != nil {
			return err
		}
	}
	expected := map[string]bool{".": true}
	for _, name := range coreChecks {
		expected[name+".d"] = true
		expected[filepath.Join(name+".d", "conf.yaml")] = true
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if !expected[rel] {
			return fmt.Errorf("package-core check inventory contains an unexpected path")
		}
		delete(expected, rel)
		if filepath.Base(path) != "conf.yaml" {
			if !d.IsDir() {
				return fmt.Errorf("package-core check directory must not be a symlink")
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("package-core check must be a regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != coreCheckConfig {
			return fmt.Errorf("package-core check contents changed")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(expected) > 0 {
		return fmt.Errorf("package-core check inventory is incomplete")
	}
	return nil
}

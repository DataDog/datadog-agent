// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestWriteConfigSymlinks(t *testing.T) {
	fleetDir := t.TempDir()
	userDir := t.TempDir()
	err := os.WriteFile(filepath.Join(userDir, "datadog.yaml"), []byte("user config"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(fleetDir, "datadog.yaml"), []byte("fleet config"), 0644)
	assert.NoError(t, err)
	err = os.MkdirAll(filepath.Join(fleetDir, "conf.d"), 0755)
	assert.NoError(t, err)

	err = writeConfigSymlinks(userDir, fleetDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml"))
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml.override"))
	assert.FileExists(t, filepath.Join(userDir, "conf.d.override"))
	configContent, err := os.ReadFile(filepath.Join(userDir, "datadog.yaml"))
	assert.NoError(t, err)
	overrideConfigConent, err := os.ReadFile(filepath.Join(userDir, "datadog.yaml.override"))
	assert.NoError(t, err)
	assert.Equal(t, "user config", string(configContent))
	assert.Equal(t, "fleet config", string(overrideConfigConent))

	fleetDir = t.TempDir()
	err = writeConfigSymlinks(userDir, fleetDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(userDir, "datadog.yaml"))
	assert.NoFileExists(t, filepath.Join(userDir, "datadog.yaml.override"))
	assert.NoFileExists(t, filepath.Join(userDir, "conf.d.override"))
}

func TestMerge_SimpleMap(t *testing.T) {
	base := map[string]any{
		"foo": "bar",
	}
	override := map[string]any{
		"foo": "baz",
	}
	result, err := merge(base, override)
	assert.NoError(t, err)
	expected := map[string]any{
		"foo": "baz",
	}
	assert.Equal(t, expected, result)
}

func TestMerge_MapWithListValue(t *testing.T) {
	base := map[string]any{
		"list": []any{1, 2, 3},
	}
	override := map[string]any{
		"list": []any{4, 5},
	}
	result, err := merge(base, override)
	assert.NoError(t, err)
	expected := map[string]any{
		"list": []any{4, 5},
	}
	assert.Equal(t, expected, result)
}

func TestMerge_Maps(t *testing.T) {
	base := map[string]any{
		"a": 1,
		"b": 2,
	}
	override := map[string]any{
		"b": 3,
		"c": 4,
	}
	result, err := merge(base, override)
	assert.NoError(t, err)
	expected := map[string]any{
		"a": 1,
		"b": 3,
		"c": 4,
	}
	assert.Equal(t, expected, result)
}

func TestMerge_NilBase(t *testing.T) {
	var base any = nil
	override := map[string]any{
		"foo": "bar",
	}
	result, err := merge(base, override)
	assert.NoError(t, err)
	assert.Equal(t, override, result)
}

func TestMerge_NilOverride(t *testing.T) {
	base := map[string]any{
		"foo": "bar",
	}
	var override any = nil
	result, err := merge(base, override)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestMerge_DeepMap(t *testing.T) {
	base := map[string]any{
		"a": map[string]any{
			"x": 1,
			"y": 2,
		},
	}
	override := map[string]any{
		"a": map[string]any{
			"y": 3,
			"z": 4,
		},
	}
	result, err := merge(base, override)
	assert.NoError(t, err)
	expected := map[string]any{
		"a": map[string]any{
			"x": 1,
			"y": 3,
			"z": 4,
		},
	}
	assert.Equal(t, expected, result)
}

func TestMerge_TypeMismatch(t *testing.T) {
	base := map[string]any{"a": 1}
	override := map[string]any{"a": []any{1, 2}}
	result, err := merge(base, override)
	assert.NoError(t, err)
	expected := map[string]any{"a": []any{1, 2}}
	assert.Equal(t, expected, result)
}

func TestConfigActionApply_Write(t *testing.T) {
	tmpDir := t.TempDir()
	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	path := "test.yaml"
	val := map[string]any{"foo": "bar"}
	action := &ConfigAction{
		ActionType: ConfigActionTypeWrite,
		Path:       path,
		Value:      val,
	}
	err = action.Apply(root)
	assert.NoError(t, err)

	// Check file contents
	content, err := os.ReadFile(filepath.Join(tmpDir, path))
	assert.NoError(t, err)
	var out map[string]any
	err = yaml.Unmarshal(content, &out)
	assert.NoError(t, err)
	assert.Equal(t, val, out)
}

func TestConfigActionApply_Merge(t *testing.T) {
	tmpDir := t.TempDir()
	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	path := "merge.yaml"
	// Write initial file
	initial := map[string]any{"a": 1, "b": 2}
	initialBytes, _ := yaml.Marshal(initial)
	err = os.WriteFile(filepath.Join(tmpDir, path), initialBytes, 0644)
	assert.NoError(t, err)

	override := map[string]any{"b": 3, "c": 4}
	action := &ConfigAction{
		ActionType: ConfigActionTypeMerge,
		Path:       path,
		Value:      override,
	}
	err = action.Apply(root)
	assert.NoError(t, err)

	// Check file contents
	content, err := os.ReadFile(filepath.Join(tmpDir, path))
	assert.NoError(t, err)
	var out map[string]any
	err = yaml.Unmarshal(content, &out)
	assert.NoError(t, err)
	expected := map[string]any{"a": 1, "b": 3, "c": 4}
	assert.Equal(t, expected, out)
}

func TestConfigActionApply_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	root, err := os.OpenRoot(tmpDir)
	assert.NoError(t, err)
	defer root.Close()

	path := "delete.yaml"
	_ = os.WriteFile(filepath.Join(tmpDir, path), []byte("foo: bar"), 0644)

	action := &ConfigAction{
		ActionType: ConfigActionTypeDelete,
		Path:       path,
	}
	err = action.Apply(root)
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Join(tmpDir, path))
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

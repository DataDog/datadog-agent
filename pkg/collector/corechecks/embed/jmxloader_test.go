// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package embed

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func getFile() (string, error) {
	_, fileName, _, ok := runtime.Caller(1)
	if !ok {
		return "", errors.New("could not get current (caller) file")
	}
	return fileName, nil
}

func TestLoadCheckConfig(t *testing.T) {

	tmp, err := ioutil.TempDir("", "datadog-agent")
	if err != nil {
		t.Fatalf("unable to create temporary directory: %v", err)
	}

	defer os.RemoveAll(tmp) // clean up

	config.Datadog.Set("jmx_pipe_path", tmp)

	jl, err := NewJMXCheckLoader()
	assert.Nil(t, err)
	assert.NotNil(t, jl)

	f, err := getFile()
	if err != nil {
		t.FailNow()
	}

	d := filepath.Dir(f)

	paths := []string{filepath.Join(d, "fixtures/")}
	fp := providers.NewFileConfigProvider(paths)
	assert.NotNil(t, fp)

	cfgs, err := fp.Collect()
	assert.Nil(t, err)

	// should be three valid instances
	assert.Len(t, cfgs, 4)
	for _, cfg := range cfgs {
		_, err := jl.Load(cfg)
		assert.Nil(t, err)

	}

	for _, cfg := range cfgs {
		found := false
		for k := range jmxLauncher.checks {
			if k == fmt.Sprintf("%s.yaml", cfg.Name) {
				found = true
				break
			}
		}
		assert.True(t, found)
	}
}

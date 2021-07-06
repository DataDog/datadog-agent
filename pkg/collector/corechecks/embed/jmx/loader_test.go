// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build jmx

package jmx

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
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
	ctx := context.Background()

	tmp, err := ioutil.TempDir("", "datadog-agent")
	if err != nil {
		t.Fatalf("unable to create temporary directory: %v", err)
	}

	defer os.RemoveAll(tmp) // clean up

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

	cfgs, err := fp.Collect(ctx)
	assert.Nil(t, err)
	assert.Len(t, cfgs, 5)

	checks := []check.Check{}
	numOtherInstances := 0

	for _, cfg := range cfgs {
		for _, instance := range cfg.Instances {
			if loadedCheck, err := jl.Load(cfg, instance); err == nil {
				checks = append(checks, loadedCheck)
			} else {
				numOtherInstances++
			}
		}
	}

	// should be five valid JMX instances and one non-JMX instance
	assert.Len(t, checks, 5)
	assert.Equal(t, numOtherInstances, 2)

	for _, cfg := range cfgs {
		found := false
		for _, c := range checks {
			if c.String() == cfg.Name {
				found = true
				break
			}
		}
		assert.True(t, found)
	}
}

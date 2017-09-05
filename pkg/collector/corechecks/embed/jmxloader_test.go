// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build jmx,disabled

package embed

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

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

	d := filepath.Dir(f)

	paths := []string{filepath.Join(d, "fixtures/")}
	fp := providers.NewFileConfigProvider(paths)
	assert.NotNil(t, fp)

	cfgs, err := fp.Collect()
	assert.Nil(t, err)

	// should be two valid instances
	assert.Len(t, cfgs, 2)
	for _, cfg := range cfgs {
		// parallel because reader/writers block
		go func(c check.Config) {
			_, err := jl.Load(cfg)
			assert.Nil(t, err)
		}(cfg)
	}
}

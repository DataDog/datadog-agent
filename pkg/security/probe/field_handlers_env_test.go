// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestResolveProcessEnv(t *testing.T) {
	handlers := map[string]func(*model.Event, *model.Process) map[string]string{
		"ebpf":     (&EBPFFieldHandlers{}).ResolveProcessEnv,
		"ebpfless": (&EBPFLessFieldHandlers{}).ResolveProcessEnv,
	}

	for name, resolve := range handlers {
		t.Run(name, func(t *testing.T) {
			// read from the environment entry of the process
			process := &model.Process{
				EnvsEntry: &model.EnvsEntry{Values: []string{"PATH=/bin"}},
				Env:       map[string]string{"PATH": "/usr/bin"},
			}
			assert.Equal(t, map[string]string{"PATH": "/bin"}, resolve(nil, process))

			// without one, the field set on the process, as the other environment fields
			process = &model.Process{Env: map[string]string{"PATH": "/usr/bin"}}
			assert.Equal(t, map[string]string{"PATH": "/usr/bin"}, resolve(nil, process))

			assert.Nil(t, resolve(nil, &model.Process{}))
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailerfactory

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// fakeRegistry implements auditor.Registry.
type fakeRegistry struct {
	t                  *testing.T
	expectedRegistryID string
	containerInReg     bool
}

// GetOffset implements auditor.Registry#GetOffset.
func (r *fakeRegistry) GetOffset(identifier string) string {
	require.Equal(r.t, r.expectedRegistryID, identifier)
	if r.containerInReg {
		return "from this time yesterday"
	}
	return ""
}

// GetTailingMode implements auditor.Registry#GetTailingMode.
func (r *fakeRegistry) GetTailingMode(identifier string) string {
	panic("unused")
}

func TestUseFile(t *testing.T) {
	ctrs := containersorpods.LogContainers
	pods := containersorpods.LogPods
	cases := []struct {
		name string // name is the test case name.

		logWhat        containersorpods.LogWhat // logWhat sets the containersOrPods result.
		dcuf           bool                     // dcuf sets logs_config.docker_container_use_file.
		dcfuf          bool                     // dcuf sets logs_config.docker_container_force_use_file.
		kcuf           bool                     // kcuf sets logs_config.k8s_container_use_file.
		containerInReg bool                     // containerInReg sets presence of a socket registry entry
		useFileResult  bool                     // expected result
	}{
		// ⚠ below signifies that the result is surprising for users but matches existing behavior
		{logWhat: ctrs, dcuf: false, dcfuf: false, containerInReg: false, useFileResult: false},
		{logWhat: ctrs, dcuf: false, dcfuf: false, containerInReg: true, useFileResult: false},
		{logWhat: ctrs, dcuf: false, dcfuf: true, containerInReg: false, useFileResult: false}, // ⚠
		{logWhat: ctrs, dcuf: false, dcfuf: true, containerInReg: true, useFileResult: false},  // ⚠
		{logWhat: ctrs, dcuf: true, dcfuf: false, containerInReg: false, useFileResult: true},
		{logWhat: ctrs, dcuf: true, dcfuf: false, containerInReg: true, useFileResult: false},
		{logWhat: ctrs, dcuf: true, dcfuf: true, containerInReg: false, useFileResult: true},
		{logWhat: ctrs, dcuf: true, dcfuf: true, containerInReg: true, useFileResult: true},
		{logWhat: pods, kcuf: false, useFileResult: false},
		{logWhat: pods, kcuf: true, useFileResult: true},
	}
	for _, c := range cases {
		name := fmt.Sprintf("logWhat=%s/dcuf=%t/dcfuf=%t/kcuf=%t/containerInReg=%t",
			c.logWhat.String(), c.dcuf, c.dcfuf, c.kcuf, c.containerInReg)
		t.Run(name, func(t *testing.T) {
			runtime := "evrgivn"
			identifier := "abc123"

			cfg := coreConfig.Mock(t)
			cfg.Set("logs_config.docker_container_use_file", c.dcuf)
			cfg.Set("logs_config.docker_container_force_use_file", c.dcfuf)
			cfg.Set("logs_config.k8s_container_use_file", c.kcuf)

			reg := &fakeRegistry{
				t:                  t,
				expectedRegistryID: fmt.Sprintf("%s:%s", runtime, identifier),
				containerInReg:     c.containerInReg,
			}

			tf := &factory{
				cop:      containersorpods.NewDecidedChooser(c.logWhat),
				registry: reg,
			}

			source := &sources.LogSource{
				Config: &config.LogsConfig{
					Type:       runtime,
					Identifier: identifier,
				},
			}

			result := tf.useFile(source)
			require.Equal(t, c.useFileResult, result)
		})
	}

}

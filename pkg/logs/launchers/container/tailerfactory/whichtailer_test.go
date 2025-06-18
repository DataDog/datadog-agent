// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package tailerfactory

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/util/containersorpods"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"

	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
)

func TestWhichTailer(t *testing.T) {
	ctrs := containersorpods.LogContainers
	pods := containersorpods.LogPods
	cases := []struct {
		name string // name is the test case name.

		logWhat        containersorpods.LogWhat // logWhat sets the containersOrPods result.
		dcuf           bool                     // dcuf sets logs_config.docker_container_use_file.
		dcfuf          bool                     // dcuf sets logs_config.docker_container_force_use_file.
		kcuf           bool                     // kcuf sets logs_config.k8s_container_use_file.
		kcua           bool                     // kcua sets logs_config.k8s_container_use_kubelet_api.
		containerInReg bool                     // containerInReg sets presence of a socket registry entry
		tailer         whichTailer              // expected result
	}{
		// ⚠ below signifies that the result is surprising for users but matches existing behavior
		{logWhat: ctrs, dcuf: false, dcfuf: false, containerInReg: false, tailer: socket},
		{logWhat: ctrs, dcuf: false, dcfuf: false, containerInReg: true, tailer: socket},
		{logWhat: ctrs, dcuf: false, dcfuf: true, containerInReg: false, tailer: socket}, // ⚠
		{logWhat: ctrs, dcuf: false, dcfuf: true, containerInReg: true, tailer: socket},  // ⚠
		{logWhat: ctrs, dcuf: true, dcfuf: false, containerInReg: false, tailer: file},
		{logWhat: ctrs, dcuf: true, dcfuf: false, containerInReg: true, tailer: socket},
		{logWhat: ctrs, dcuf: true, dcfuf: true, containerInReg: false, tailer: file},
		{logWhat: ctrs, dcuf: true, dcfuf: true, containerInReg: true, tailer: file},
		{logWhat: pods, kcua: true, kcuf: true, tailer: api}, // k8s_container_use_file supersedes k8s_container_use_file
		{logWhat: pods, kcuf: false, tailer: socket},
		{logWhat: pods, kcuf: true, tailer: file},
	}
	for _, c := range cases {
		name := fmt.Sprintf("logWhat=%s/dcuf=%t/dcfuf=%t/kcuf=%t/nativeLogging=%t/containerInReg=%t",
			c.logWhat.String(), c.dcuf, c.dcfuf, c.kcuf, c.kcua, c.containerInReg)
		t.Run(name, func(t *testing.T) {
			runtime := "evrgivn"
			identifier := "abc123"

			cfg := configmock.New(t)
			cfg.SetWithoutSource("logs_config.docker_container_use_file", c.dcuf)
			cfg.SetWithoutSource("logs_config.docker_container_force_use_file", c.dcfuf)
			cfg.SetWithoutSource("logs_config.k8s_container_use_kubelet_api", c.kcua)
			cfg.SetWithoutSource("logs_config.k8s_container_use_file", c.kcuf)

			registryID := fmt.Sprintf("%s:%s", runtime, identifier)

			reg := auditorMock.NewMockRegistry()
			if c.containerInReg {
				reg.StoredOffsets[registryID] = "from this time yesterday"
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
				Messages: config.NewMessages(),
			}

			result := tf.whichTailer(source)
			require.Equal(t, c.tailer, result)
		})
	}

}

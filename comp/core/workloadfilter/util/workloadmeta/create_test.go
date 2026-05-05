// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestCreateProcess_TcpPorts(t *testing.T) {
	t.Run("populates TcpPorts from Service.TCPPorts", func(t *testing.T) {
		p := CreateProcess(&workloadmeta.Process{
			Name:    "redis-server",
			Cmdline: []string{"/usr/bin/redis-server"},
			Service: &workloadmeta.Service{
				TCPPorts: []uint16{6379, 6380},
			},
		})
		assert.Equal(t, []int32{6379, 6380}, p.GetTcpPorts())
	})

	t.Run("returns empty TcpPorts when Service is nil", func(t *testing.T) {
		p := CreateProcess(&workloadmeta.Process{
			Name:    "no-service",
			Cmdline: []string{"no-service"},
			Service: nil,
		})
		assert.Empty(t, p.GetTcpPorts())
	})

	t.Run("returns empty TcpPorts when Service.TCPPorts is empty", func(t *testing.T) {
		p := CreateProcess(&workloadmeta.Process{
			Name:    "no-ports",
			Cmdline: []string{"no-ports"},
			Service: &workloadmeta.Service{},
		})
		assert.Empty(t, p.GetTcpPorts())
	})
}

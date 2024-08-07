// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
)

func Test_shouldScheduleNetworkPathForConn(t *testing.T) {
	tests := []struct {
		name           string
		conn           *model.Connection
		shouldSchedule bool
	}{
		{
			name: "should schedule",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
			},
			shouldSchedule: true,
		},
		{
			name: "should not schedule incoming conn",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_incoming,
				Family:    model.ConnectionFamily_v4,
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule conn with none direction",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_none,
				Family:    model.ConnectionFamily_v4,
			},
			shouldSchedule: false,
		},
		{
			name: "should not schedule ipv6",
			conn: &model.Connection{
				Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
				Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
				Direction: model.ConnectionDirection_outgoing,
				Family:    model.ConnectionFamily_v6,
			},
			shouldSchedule: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.shouldSchedule, shouldScheduleNetworkPathForConn(tt.conn))
		})
	}
}

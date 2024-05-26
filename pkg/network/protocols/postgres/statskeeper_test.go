// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestStatKeeperProcess(t *testing.T) {
	cfg := config.New()
	cfg.MaxPostgresStatsBuffered = 100
	s := NewStatkeeper(cfg)
	for i := 0; i < 20; i++ {
		s.Process(&EventWrapper{
			EbpfEvent: &EbpfEvent{
				Tx: EbpfTx{
					Request_started:    1,
					Response_last_seen: 10,
				},
			},
			operationSet: true,
			operation:    SelectOP,
			tableNameSet: true,
			tableName:    "dummy",
		})
	}

	require.Equal(t, 1, len(s.stats))
	for k, stat := range s.stats {
		require.Equal(t, "dummy", k.TableName)
		require.Equal(t, SelectOP, k.Operation)
		require.Equal(t, 20, stat.Count)
		require.Equal(t, float64(20), stat.Latencies.GetCount())
	}
}

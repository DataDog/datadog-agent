// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectorimpl

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/collector/collector/impl/internal/middleware"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type inspectedCheck struct {
	stub.StubCheck
	snapshot sharding.CheckSnapshot
}

func (c *inspectedCheck) KSMShardingSnapshot() sharding.CheckSnapshot { return c.snapshot }

func TestKSMShardingEndpointInspectsWrappedChecks(t *testing.T) {
	c := &collectorImpl{checks: make(map[checkid.ID]*middleware.CheckWrapper)}
	for _, id := range []string{"ksm:z", "ksm:a"} {
		ch := &inspectedCheck{snapshot: sharding.CheckSnapshot{CheckID: id, State: "active", Stores: []sharding.StoreInfo{}}}
		c.checks[checkid.ID(id)] = middleware.NewCheckWrapper(ch, nil, option.None[agenttelemetry.Component](), option.None[healthplatform.Component]())
	}
	c.checks["other"] = middleware.NewCheckWrapper(&stub.StubCheck{}, nil, option.None[agenttelemetry.Component](), option.None[healthplatform.Component]())
	w := httptest.NewRecorder()
	c.writeKSMSharding(w, httptest.NewRequest("GET", "/ksm-sharding", nil))
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var snapshots []sharding.CheckSnapshot
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &snapshots))
	require.Len(t, snapshots, 2)
	require.Equal(t, "ksm:a", snapshots[0].CheckID)
	require.Equal(t, "ksm:z", snapshots[1].CheckID)
}

func TestKSMShardingEndpointBeforeChecksAreLoaded(t *testing.T) {
	c := &collectorImpl{}
	w := httptest.NewRecorder()
	c.writeKSMSharding(w, httptest.NewRequest("GET", "/ksm-sharding", nil))
	require.Equal(t, "[]", w.Body.String())
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ksmsharding

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPredictCommandOffline(t *testing.T) {
	cmd := Commands(&command.GlobalParams{})[0]
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"predict", "--json", "--shard-count", "3", "--shard-criteria", "namespace", "--namespace", "datadog", "--resource", "core/Pod", "--shard-id", "2"})
	require.NoError(t, cmd.Execute())
	var result prediction
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	require.Equal(t, sharding.HashKey("datadog"), result.HashKey)
	require.Equal(t, 2, result.OwnerShard)
	require.NotNil(t, result.OwnedByShard)
	require.True(t, *result.OwnedByShard)
}

func TestPredictColocationAndClusterScope(t *testing.T) {
	pod, err := predictOwnership(predictionParams{count: 3, criteria: []string{"resource"}, resource: "core/Pod", colocatedResource: "pods", shardID: -1})
	require.NoError(t, err)
	node, err := predictOwnership(predictionParams{count: 3, criteria: []string{"resource"}, resource: "core/Node", colocatedResource: "pods", shardID: -1})
	require.NoError(t, err)
	require.Equal(t, sharding.HashKey("|pods"), pod.HashKey)
	require.Equal(t, pod.OwnerShard, node.OwnerShard)
	require.Empty(t, node.Namespace)
}

func TestPredictRejectsInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"--shard-count", "0", "--shard-criteria", "namespace", "--resource", "core/Pod"},
		{"--shard-count", "3", "--resource", "core/Pod"},
		{"--shard-count", "3", "--shard-criteria", "unknown", "--resource", "core/Pod"},
		{"--shard-count", "3", "--shard-criteria", "namespace,namespace", "--resource", "core/Pod"},
		{"--shard-count", "3", "--shard-criteria", "namespace", "--resource", "pods"},
		{"--shard-count", "3", "--shard-criteria", "namespace", "--resource", "core/Pod", "--shard-id", "3"},
		{"--shard-count", "3", "--shard-criteria", "namespace", "--resource", "core/Pod", "--colocated-resource", "pods"},
	} {
		cmd := Commands(&command.GlobalParams{})[0]
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(append([]string{"predict"}, args...))
		require.Error(t, cmd.Execute(), "args: %v", args)
	}
}

func TestStoresCommandWiring(t *testing.T) {
	fxutil.TestOneShotSubcommand(t, Commands(&command.GlobalParams{}),
		[]string{"ksm-sharding", "stores", "--json", "--check-id", "ksm:1"}, runStores,
		func(params *storesParams, _ core.BundleParams) {
			require.True(t, params.json)
			require.Equal(t, "ksm:1", params.checkID)
		})
}

func TestStoresIPC(t *testing.T) {
	cfg := config.NewMock(t)
	logger := logmock.New(t)
	client := ipcmock.New(t)
	server := client.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/ksm-sharding" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]sharding.CheckSnapshot{{CheckID: "ksm:1", State: "active", Stores: []sharding.StoreInfo{}}, {CheckID: "ksm:2", State: "initializing", Stores: []sharding.StoreInfo{}}})
	}))
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	cfg.Set("cmd_host", serverURL.Hostname(), model.SourceFile)
	cfg.Set("cmd_port", serverURL.Port(), model.SourceFile)
	var output bytes.Buffer
	params := &storesParams{output: &output, json: true, checkID: "ksm:1"}
	require.NoError(t, runStores(logger, params, client.GetClient()))
	var result []sharding.CheckSnapshot
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	require.Len(t, result, 1)
	require.Equal(t, "ksm:1", result[0].CheckID)
	params.checkID = "missing"
	require.ErrorContains(t, runStores(logger, params, client.GetClient()), "was not found")
}

func TestPrintStoresDistinguishesStates(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, printStores(&output, []sharding.CheckSnapshot{{CheckID: "ksm:1", State: "initializing"}, {CheckID: "ksm:2", State: "eager"}}))
	require.Contains(t, output.String(), "initializing")
	require.Contains(t, output.String(), "uses eager collection")
	output.Reset()
	require.NoError(t, printStores(&output, nil))
	require.Contains(t, output.String(), "No KSM checks are loaded")
}

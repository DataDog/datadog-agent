// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/contexttop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"dogstatsd", "top"},
		topContexts,
		func(f *topFlags) {
			assert.Equal(t, "", f.path)
		})
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"dogstatsd", "dump-contexts"},
		dumpContexts,
		func() {},
	)
}

func TestTopFromPathJSONDoesNotStartAgentComponents(t *testing.T) {
	dump, err := os.CreateTemp(t.TempDir(), "dogstatsd-contexts-*.json")
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(dump).Encode(aggregator.ContextDebugRepr{
		Name:       "requests",
		MetricTags: []string{"env:prod", "endpoint:/health"},
	}))
	require.NoError(t, dump.Close())

	cmd := Commands(&command.GlobalParams{})[0]
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"top", "--path", dump.Name(), "--json", "-m", "1", "-t", "2"})
	require.NoError(t, cmd.Execute())

	var result contexttop.Result
	require.NoError(t, json.Unmarshal(output.Bytes(), &result))
	require.Equal(t, contexttop.Result{Metrics: []contexttop.Metric{{
		Name:     "requests",
		Contexts: 1,
		Tags: []contexttop.Tag{
			{Key: "endpoint", UniqueValues: 1},
			{Key: "env", UniqueValues: 1},
		},
	}}}, result)
}

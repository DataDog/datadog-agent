// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package milvus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

func randomTestID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback that still produces a valid tag value.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// milvusSuite drives the Milvus E2E scenario: it provisions the environment
// (see provisioner.go), then asserts Milvus integration metrics appear in
// the real Datadog intake tagged with the per-run e2e_test_id.
type milvusSuite struct {
	e2e.BaseSuite[Env]

	testID string
}

// TestMilvusE2E provisions Milvus + a traffic generator + a host Agent and
// verifies that the Milvus integration metrics reach the real Datadog intake.
//
// Notes:
//   - No fakeintake is used: the agent forwards to datadoghq.com using the
//     API key from the runner configuration.
//   - The test scopes its queries with a unique `e2e_test_id` tag so multiple
//     concurrent runs don't interfere.
func TestMilvusE2E(t *testing.T) {
	t.Parallel()

	testID := randomTestID()
	s := &milvusSuite{testID: testID}

	e2e.Run(t, s,
		e2e.WithPulumiProvisioner(EnvProvisioner(testID), nil),
		e2e.WithStackName(fmt.Sprintf("milvus-e2e-%s", testID)),
	)
}

// TestAgentReady is a fast smoke check that the host Agent is up and the
// Milvus integration was scheduled.
func (s *milvusSuite) TestAgentReady() {
	require.True(s.T(), s.Env().Agent.Client.IsReady())

	s.EventuallyWithT(func(c *assert.CollectT) {
		out := s.Env().Agent.Client.Status().Content
		require.Contains(c, out, "milvus", "milvus integration should be loaded in agent status")
	}, 5*time.Minute, 15*time.Second)
}

// TestMilvusMetricsReachRealIntake polls the real Datadog backend for
// Milvus metrics tagged with our run's e2e_test_id. The Milvus integration
// emits `milvus.proxy.num_collections` (a fundamental gauge); we use it as
// a canary that the entire pipeline is wired up:
//
//	traffic -> milvus -> /metrics -> agent (milvus.d) -> intake -> backend
func (s *milvusSuite) TestMilvusMetricsReachRealIntake() {
	client := newDatadogAPIClient(s.T())

	query := fmt.Sprintf("avg:milvus.proxy.num_collections{e2e_test_id:%s}", s.testID)

	// Milvus needs ~1-2 minutes to start, the traffic generator a bit longer
	// to install pymilvus, and the backend takes ~1 minute to surface fresh
	// series in the metrics query API. Use a generous bound.
	s.EventuallyWithT(func(c *assert.CollectT) {
		now := time.Now()
		api := datadogV1.NewMetricsApi(client.api)
		resp, httpResp, err := api.QueryMetrics(
			client.ctx,
			now.Add(-5*time.Minute).Unix(),
			now.Unix(),
			query,
		)
		if httpResp != nil {
			_ = httpResp.Body.Close()
		}
		require.NoError(c, err, "query metrics failed")
		require.NotEmpty(c, resp.Series, "no series returned yet for %q", query)
		require.NotEmpty(c, resp.Series[0].Pointlist, "no datapoints yet for %q", query)
	}, 15*time.Minute, 30*time.Second)
}

// datadogAPIClient is a minimal v2 Datadog API client built from the same
// runner-provided credentials the Agent itself uses. It lets us assert
// against the real intake.
type datadogAPIClient struct {
	api *datadog.APIClient
	ctx context.Context
}

func newDatadogAPIClient(t require.TestingT) *datadogAPIClient {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(t, err)
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(t, err)

	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {Key: apiKey},
			"appKeyAuth": {Key: appKey},
		},
	)
	cfg := datadog.NewConfiguration()
	return &datadogAPIClient{
		api: datadog.NewAPIClient(cfg),
		ctx: ctx,
	}
}

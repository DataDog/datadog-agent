// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clusterinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TestEKSIdentityFromAnOlderClusterAgent runs the real token middleware of a Cluster Agent built
// before the identity endpoint: the route is missing and the path is not in its external path
// set. The node's Cluster Agent token is then checked as an intra-pod token first, which fails
// with 403 "invalid session token" before routing, rather than the 404 of the missing route. The
// node must take that as a final answer: no retry at startup, an empty success for host tags,
// and no Warn on every refresh.
func TestEKSIdentityFromAnOlderClusterAgent(t *testing.T) {
	const token = "01234567890123456789012345678901"
	cfg := configmock.New(t)
	cfg.SetInTest("cluster_agent.auth_token", token)
	cfg.SetInTest("cluster_agent.enabled", true)
	ipcComp := ipcmock.New(t)

	olderIsExternalPath := func(path string) bool {
		return path != "/api/v1/cluster/eks-identity" && isExternalPath(path)
	}
	var identityRequests atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Major":7,"Minor":60,"Patch":0}`))
	})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/cluster/eks-identity" {
			identityRequests.Add(1)
		}
		validateTokenFor(ipcComp, olderIsExternalPath, func() string { return token })(mux).ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	cfg.SetInTest("cluster_agent.url", srv.URL)
	clusteragent.ResetGlobalClusterAgentClientForTest()
	t.Cleanup(clusteragent.ResetGlobalClusterAgentClientForTest)
	clusterinfo.SetupStartupForTest(t, 3*time.Second, true)

	var buf bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(&buf, log.WarnLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "warn")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "off") })

	client, err := clusteragent.GetClusterAgentClient()
	require.NoError(t, err)
	_, err = client.GetEKSClusterIdentity(t.Context())
	var statusErr *clusteragent.StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusForbidden, statusErr.StatusCode, "an older Cluster Agent answers 403, not 404")
	assert.False(t, statusErr.RetryLater)

	start := time.Now()
	_, err = clusterinfo.GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
	require.Error(t, err)
	assert.Less(t, time.Since(start), time.Second, "final at startup")

	for range 3 {
		tags, err := clusterinfo.GetClusterAgentEKSIdentityHostTags(t.Context())
		require.NoError(t, err, "an empty success for host tags")
		assert.Empty(t, tags)
	}
	log.Flush()
	assert.NotContains(t, buf.String(), "WARN")
	assert.Equal(t, int32(5), identityRequests.Load())
}

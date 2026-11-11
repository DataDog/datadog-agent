// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && kubeapiserver

package privateactionrunnerimpl

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/create"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

const (
	rolloutKeyA = "api-key-a"
	rolloutKeyB = "api-key-b"

	rolloutPollInterval = 10 * time.Millisecond
	rolloutSecretName   = "par-identity"
	rolloutNamespace    = "par-rollout-test"
)

var rolloutOrgs = map[string]int{rolloutKeyA: 1, rolloutKeyB: 2}

type rolloutLeader struct {
	leader atomic.Bool
	notify chan struct{}
}

func newRolloutLeader(isLeader bool) *rolloutLeader {
	l := &rolloutLeader{notify: make(chan struct{}, 1)}
	l.leader.Store(isLeader)
	return l
}

func (l *rolloutLeader) IsLeader() bool { return l.leader.Load() }

func (l *rolloutLeader) Subscribe() (<-chan struct{}, func() bool) { return l.notify, l.IsLeader }

func (l *rolloutLeader) becomeLeader() {
	l.leader.Store(true)
	select {
	case l.notify <- struct{}{}:
	default:
	}
}

type rolloutResult struct {
	cfg *parconfig.Config
	err error
}

type rolloutReplica struct {
	t      *testing.T
	ctx    context.Context
	runner *PrivateActionRunner
	leader *rolloutLeader
}

func (r *rolloutReplica) start() <-chan rolloutResult {
	ch := make(chan rolloutResult, 1)
	go func() {
		cfg, err := r.runner.getRunnerConfig(r.ctx)
		ch <- rolloutResult{cfg, err}
	}()
	return ch
}

func (r *rolloutReplica) resolve() rolloutResult {
	r.t.Helper()
	return awaitRollout(r.t, r.start())
}

func awaitRollout(t *testing.T, ch <-chan rolloutResult) rolloutResult {
	t.Helper()
	select {
	case res := <-ch:
		require.NoError(t, res.err)
		return res
	case <-time.After(5 * time.Second):
		require.FailNow(t, "replica did not resolve its identity")
		return rolloutResult{}
	}
}

func assertRolloutPending(t *testing.T, ch <-chan rolloutResult) {
	t.Helper()
	select {
	case res := <-ch:
		require.FailNow(t, "replica resolved its identity before it could", "result: %+v", res)
	case <-time.After(20 * rolloutPollInterval):
	}
}

type rolloutCluster struct {
	t           *testing.T
	ctx         context.Context
	client      *fake.Clientset
	opmsURL     string
	mu          sync.Mutex
	enrollments map[string]int
}

func newRolloutCluster(t *testing.T) *rolloutCluster {
	t.Helper()
	coreconfig.NewMock(t).SetInTest("kube_resources_namespace", rolloutNamespace)
	flavor.SetTestFlavor(t, flavor.ClusterAgent)
	clusterIDKey := cache.BuildAgentKey("orchestratorClusterID")
	cache.Cache.Set(clusterIDKey, "test-cluster-id", cache.NoExpiration)
	t.Cleanup(func() { cache.Cache.Delete(clusterIDKey) })
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	c := &rolloutCluster{t: t, ctx: ctx, client: fake.NewSimpleClientset(), enrollments: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("DD-API-KEY")
		c.mu.Lock()
		c.enrollments[key]++
		n := c.enrollments[key]
		c.mu.Unlock()
		fmt.Fprintf(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":%d,"runner_id":"runner-%s-%d"}}}`, rolloutOrgs[key], key, n)
	}))
	t.Cleanup(srv.Close)
	c.opmsURL = srv.URL
	return c
}

// newReplicaConfig returns a config private to one replica: the mock config is a process-wide singleton,
// and replicas must not share the api_key or the identity set at runtime.
func newReplicaConfig(overrides map[string]interface{}) model.BuildableConfig {
	conf := create.NewConfig("datadog")
	conf.BuildSchema()
	conf.SetTestOnlyDynamicSchema(true)
	for k, v := range overrides {
		conf.SetInTest(k, v)
	}
	return conf
}

func (c *rolloutCluster) newReplica(apiKey string, selfEnroll, isLeader bool, extra map[string]interface{}) *rolloutReplica {
	overrides := map[string]interface{}{
		"api_key":                           apiKey,
		"dd_url":                            c.opmsURL,
		"cluster_name":                      "test-cluster",
		"private_action_runner.enabled":     true,
		"private_action_runner.self_enroll": selfEnroll,
		"private_action_runner.api_key_only_enrollment": true,
		"private_action_runner.identity_secret_name":    rolloutSecretName,
	}
	maps.Copy(overrides, extra)
	cfg := newReplicaConfig(overrides)
	leader := newRolloutLeader(isLeader)
	hostname, _ := hostnamemock.NewMock("test-pod")
	return &rolloutReplica{
		t:      c.t,
		ctx:    c.ctx,
		leader: leader,
		runner: &PrivateActionRunner{
			coreConfig:      cfg,
			hostnameGetter:  hostname,
			logger:          logmock.New(c.t),
			identityStorage: enrollment.NewK8sIdentityStorage(cfg, leader, c.client, rolloutPollInterval),
		},
	}
}

func (c *rolloutCluster) enrolled(apiKey string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enrollments[apiKey]
}

func (c *rolloutCluster) secretAPIKeyHash() string {
	secret, err := c.client.CoreV1().Secrets(rolloutNamespace).Get(c.ctx, rolloutSecretName, metav1.GetOptions{})
	require.NoError(c.t, err)
	return string(secret.Data["api_key_hash"])
}

func rolloutOrgID(t *testing.T, urn string) int64 {
	t.Helper()
	parts, err := util.ParseRunnerURN(urn)
	require.NoError(t, err)
	return int64(parts.OrgID)
}

func TestClusterAgentRolloutWithAPIKeyChange(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, c *rolloutCluster)
	}{
		{
			name: "one replica, new pod waits for the old one to release the lease",
			run: func(t *testing.T, c *rolloutCluster) {
				old := c.newReplica(rolloutKeyA, true, true, nil).resolve()
				require.Equal(t, 1, c.enrolled(rolloutKeyA))

				newPod := c.newReplica(rolloutKeyB, true, false, nil)
				pending := newPod.start()
				assertRolloutPending(t, pending)

				newPod.leader.becomeLeader()
				res := awaitRollout(t, pending)

				assert.Equal(t, 1, c.enrolled(rolloutKeyB))
				assert.Equal(t, enrollment.HashAPIKey(rolloutKeyB), c.secretAPIKeyHash())
				assert.EqualValues(t, rolloutOrgs[rolloutKeyB], rolloutOrgID(t, res.cfg.Urn))
				assert.NotEqual(t, old.cfg.Urn, res.cfg.Urn)
			},
		},
		{
			name: "two replicas, lease held by an old replica first",
			run: func(t *testing.T, c *rolloutCluster) {
				c.newReplica(rolloutKeyA, true, true, nil).resolve()
				require.Equal(t, 1, c.enrolled(rolloutKeyA))

				first := c.newReplica(rolloutKeyB, true, false, nil)
				second := c.newReplica(rolloutKeyB, true, false, nil)
				firstPending, secondPending := first.start(), second.start()
				assertRolloutPending(t, firstPending)
				assertRolloutPending(t, secondPending)

				first.leader.becomeLeader()
				firstRes, secondRes := awaitRollout(t, firstPending), awaitRollout(t, secondPending)

				assert.Equal(t, 1, c.enrolled(rolloutKeyB))
				assert.Equal(t, enrollment.HashAPIKey(rolloutKeyB), c.secretAPIKeyHash())
				assert.Equal(t, firstRes.cfg.Urn, secondRes.cfg.Urn)
				assert.EqualValues(t, rolloutOrgs[rolloutKeyB], rolloutOrgID(t, firstRes.cfg.Urn))
			},
		},
		{
			name: "self-enrollment disabled, configured identity wins over the stale secret",
			run: func(t *testing.T, c *rolloutCluster) {
				c.newReplica(rolloutKeyA, true, true, nil).resolve()
				require.Equal(t, 1, c.enrolled(rolloutKeyA))

				privateKey, _, err := util.GenerateKeys()
				require.NoError(t, err)
				encodedKey, err := privateKey.MarshalJSON()
				require.NoError(t, err)
				configuredURN := util.MakeRunnerURN("us1", 123, "configured-runner")

				res := c.newReplica(rolloutKeyB, false, false, map[string]interface{}{
					"private_action_runner.urn":         configuredURN,
					"private_action_runner.private_key": base64.RawURLEncoding.EncodeToString(encodedKey),
				}).resolve()

				assert.Equal(t, configuredURN, res.cfg.Urn)
				assert.Zero(t, c.enrolled(rolloutKeyB))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, newRolloutCluster(t))
		})
	}
}

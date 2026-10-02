// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenthealth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

const (
	remotePersistenceIssueName      = "Invalid Config"
	remotePersistenceIssueType      = "invalid_config"
	remotePersistenceIssueIDPrefix  = "invalid-config:"
	remotePersistenceDaemonSetName  = "dda-linux-datadog"
	remotePersistenceClusterIDMap   = "datadog-cluster-id"
	remotePersistenceContentType    = "application/vnd.api+json"
	remotePersistenceControlTimeout = 2 * time.Minute
)

type remoteBackendIssue struct {
	IssueID   string `json:"issue_id"`
	IssueName string `json:"issue_name"`
	IssueType string `json:"issue_type"`
}

type remoteBackendRequest struct {
	ResourceID          string `json:"resource_id"`
	AgentType           string `json:"agent_type"`
	APIKeyPresent       bool   `json:"api_key_present"`
	Accept              string `json:"accept"`
	AgentVersionPresent bool   `json:"agent_version_present"`
	UserAgent           string `json:"user_agent"`
}

type remoteBackendRequests struct {
	Requests []remoteBackendRequest `json:"requests"`
}

type remotePersistenceStartup struct {
	nodeResourceID    string
	clusterResourceID string
	nodeRequest       remoteBackendRequest
	clusterRequest    remoteBackendRequest
	nodeStaleIssue    *healthplatform.Issue
	clusterStaleIssue *healthplatform.Issue
	activeNodeIssue   *healthplatform.Issue
}

func (suite *admissionProbeSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
	defer suite.CleanupOnSetupFailure()

	suite.remotePersistenceStartup = suite.captureRemotePersistenceStartup()
}

func (suite *admissionProbeSuite) TestRemotePersistenceReconciliation() {
	startup := suite.remotePersistenceStartup

	suite.T().Run("StartupReconciliation", func(t *testing.T) {
		assertRemoteBackendRequest(t, startup.nodeRequest, startup.nodeResourceID, "node")
		assertRemoteBackendRequest(t, startup.clusterRequest, startup.clusterResourceID, "cluster")
		assertIssueState(t, startup.nodeStaleIssue, staleNodeIssueID, healthplatform.IssueState_ISSUE_STATE_RESOLVED)
		assertIssueState(t, startup.clusterStaleIssue, staleClusterIssueID, healthplatform.IssueState_ISSUE_STATE_RESOLVED)
		assertIssueState(t, startup.activeNodeIssue, startup.activeNodeIssue.GetId(), healthplatform.IssueState_ISSUE_STATE_ACTIVE)
		assert.True(t, strings.HasPrefix(startup.activeNodeIssue.GetId(), remotePersistenceIssueIDPrefix))
		assert.NotEqual(t, staleNodeIssueID, startup.activeNodeIssue.GetId())
	})

	suite.T().Run("ContinuingIssue", func(t *testing.T) {
		activeIssueID := startup.activeNodeIssue.GetId()
		suite.setRemoteBackendIssues(t, "node", []remoteBackendIssue{{
			IssueID:   activeIssueID,
			IssueName: remotePersistenceIssueName,
			IssueType: remotePersistenceIssueType,
		}})
		t.Cleanup(func() {
			suite.setRemoteBackendIssues(t, "node", []remoteBackendIssue{{
				IssueID:   staleNodeIssueID,
				IssueName: remotePersistenceIssueName,
				IssueType: remotePersistenceIssueType,
			}})
		})

		requestsBeforeRestart := suite.getRemoteBackendRequests(t)
		initialNodeRequestCount := countRemoteBackendRequests(requestsBeforeRestart, "node")

		suite.restartNodeAgent(t)

		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			requests, err := suite.tryGetRemoteBackendRequests()
			require.NoError(ct, err)
			nodeRequests := filterRemoteBackendRequests(requests, "node")
			require.Greater(ct, len(nodeRequests), initialNodeRequestCount, "node Agent did not reload remote issues")
			assertRemoteBackendRequest(ct, nodeRequests[len(nodeRequests)-1], startup.nodeResourceID, "node")
		}, remotePersistenceControlTimeout, 2*time.Second, "node Agent did not request its remote issue snapshot after restart")

		fakeIntake := suite.Env().FakeIntake.Client()
		payloads, err := fakeIntake.GetAgentHealth()
		require.NoError(t, err)
		assertNoResolvedIssue(t, payloads, startup.nodeResourceID, activeIssueID)

		require.NoError(t, fakeIntake.FlushServerAndResetAggregators())
		require.EventuallyWithT(t, func(ct *assert.CollectT) {
			payloads, err := fakeIntake.GetAgentHealth()
			require.NoError(ct, err)
			activeReports := countIssueReports(payloads, startup.nodeResourceID, activeIssueID, healthplatform.IssueState_ISSUE_STATE_ACTIVE)
			assert.GreaterOrEqual(ct, activeReports, 2, "expected the continuing issue in two post-restart reports")
		}, defaultIssueTimeout, 2*time.Second, "continuing issue was not repeatedly reported as ACTIVE after restart")

		payloads, err = fakeIntake.GetAgentHealth()
		require.NoError(t, err)
		assertNoResolvedIssue(t, payloads, startup.nodeResourceID, activeIssueID)
	})
}

func (suite *admissionProbeSuite) captureRemotePersistenceStartup() remotePersistenceStartup {
	ctx := context.Background()
	client := suite.Env().KubernetesCluster.Client()

	daemonSet, err := client.AppsV1().DaemonSets(clusterAgentNamespace).Get(ctx, remotePersistenceDaemonSetName, metav1.GetOptions{})
	require.NoError(suite.T(), err)
	configMaps := client.CoreV1().ConfigMaps(clusterAgentNamespace)
	clusterIDConfig, err := configMaps.Get(ctx, remotePersistenceClusterIDMap, metav1.GetOptions{})
	require.NoError(suite.T(), err)
	clusterID := clusterIDConfig.Data["id"]
	require.NotEmpty(suite.T(), clusterID)

	startup := remotePersistenceStartup{
		nodeResourceID:    string(daemonSet.UID),
		clusterResourceID: clusterID,
	}

	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		requests, err := suite.tryGetRemoteBackendRequests()
		require.NoError(ct, err)
		nodeRequest, found := findRemoteBackendRequest(requests, startup.nodeResourceID, "node")
		assert.True(ct, found, "node Agent remote issue request not observed")
		clusterRequest, found := findRemoteBackendRequest(requests, startup.clusterResourceID, "cluster")
		assert.True(ct, found, "Cluster Agent remote issue request not observed")
		startup.nodeRequest = nodeRequest
		startup.clusterRequest = clusterRequest
	}, remotePersistenceControlTimeout, 2*time.Second, "remote issue requests were not observed")

	fakeIntake := suite.Env().FakeIntake.Client()
	require.EventuallyWithT(suite.T(), func(ct *assert.CollectT) {
		payloads, err := fakeIntake.GetAgentHealth()
		require.NoError(ct, err)

		startup.nodeStaleIssue = findIssue(payloads, startup.nodeResourceID, staleNodeIssueID, healthplatform.IssueState_ISSUE_STATE_RESOLVED)
		startup.clusterStaleIssue = findIssue(payloads, startup.clusterResourceID, staleClusterIssueID, healthplatform.IssueState_ISSUE_STATE_RESOLVED)
		startup.activeNodeIssue = findIssueByPrefix(payloads, startup.nodeResourceID, remotePersistenceIssueIDPrefix, staleNodeIssueID, healthplatform.IssueState_ISSUE_STATE_ACTIVE)

		assert.NotNil(ct, startup.nodeStaleIssue, "stale node issue was not reported as RESOLVED")
		assert.NotNil(ct, startup.clusterStaleIssue, "stale cluster issue was not reported as RESOLVED")
		assert.NotNil(ct, startup.activeNodeIssue, "new node issue was not reported as ACTIVE")
	}, defaultIssueTimeout, 5*time.Second, "startup reconciliation did not reach fakeintake")

	return startup
}

func (suite *admissionProbeSuite) restartNodeAgent(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	client := suite.Env().KubernetesCluster.Client()
	daemonSet, err := client.AppsV1().DaemonSets(clusterAgentNamespace).Get(ctx, remotePersistenceDaemonSetName, metav1.GetOptions{})
	require.NoError(t, err)

	pod := findDaemonSetPod(ctx, t, daemonSet, client.CoreV1().Pods(clusterAgentNamespace))
	oldUID := pod.UID
	require.NoError(t, client.CoreV1().Pods(clusterAgentNamespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}))
	require.NoError(t, suite.Env().WaitForAgentReady(ctx,
		environments.WithLinuxNodeAgentReady(),
		environments.WithAgentReadinessTimeout(5*time.Minute),
	))

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		replacement := findDaemonSetPod(ctx, ct, daemonSet, client.CoreV1().Pods(clusterAgentNamespace))
		assert.NotEqual(ct, oldUID, replacement.UID, "node Agent pod was not replaced")
	}, time.Minute, 2*time.Second, "node Agent pod was not replaced")
}

func findDaemonSetPod(ctx context.Context, t require.TestingT, daemonSet *appsv1.DaemonSet, pods typedcorev1.PodInterface) corev1.Pod {
	selector := metav1.FormatLabelSelector(daemonSet.Spec.Selector)
	list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: selector})
	require.NoError(t, err)
	for _, pod := range list.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod
		}
	}
	require.FailNow(t, "node Agent pod not found")
	return corev1.Pod{}
}

func (suite *admissionProbeSuite) getRemoteBackendRequests(t testing.TB) []remoteBackendRequest {
	t.Helper()
	requests, err := suite.tryGetRemoteBackendRequests()
	require.NoError(t, err)
	return requests
}

func (suite *admissionProbeSuite) tryGetRemoteBackendRequests() ([]remoteBackendRequest, error) {
	body, err := suite.remoteBackendControl("GET", "/_test/requests", "")
	if err != nil {
		return nil, err
	}
	var response remoteBackendRequests
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return nil, fmt.Errorf("decode remote backend requests: %w", err)
	}
	return response.Requests, nil
}

func (suite *admissionProbeSuite) setRemoteBackendIssues(t testing.TB, agentType string, issues []remoteBackendIssue) {
	t.Helper()
	body, err := json.Marshal(map[string][]remoteBackendIssue{"issues": issues})
	require.NoError(t, err)
	_, err = suite.remoteBackendControl("PUT", "/_test/issues/"+agentType, string(body))
	require.NoError(t, err)
}

func (suite *admissionProbeSuite) remoteBackendControl(method, path, body string) (string, error) {
	ctx := context.Background()
	pods, err := suite.Env().KubernetesCluster.Client().CoreV1().Pods(remotePersistenceNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + remotePersistenceBackendName,
	})
	if err != nil {
		return "", err
	}
	if len(pods.Items) != 1 {
		return "", fmt.Errorf("expected one remote persistence backend pod, got %d", len(pods.Items))
	}

	const controlScript = `import ssl,sys,urllib.request
data = sys.argv[3].encode() if sys.argv[3] else None
request = urllib.request.Request("https://127.0.0.1:8443" + sys.argv[2], data=data, method=sys.argv[1])
request.add_header("Content-Type", "application/json")
print(urllib.request.urlopen(request, context=ssl._create_unverified_context()).read().decode())`

	stdout, stderr, err := suite.Env().KubernetesCluster.KubernetesClient.PodExec(
		remotePersistenceNamespace,
		pods.Items[0].Name,
		remotePersistenceBackendName,
		[]string{"python", "-c", controlScript, method, path, body},
	)
	if err != nil {
		return "", fmt.Errorf("remote backend control request failed: %w: %s", err, stderr)
	}
	return stdout, nil
}

func assertRemoteBackendRequest(t require.TestingT, request remoteBackendRequest, resourceID, agentType string) {
	assert.Equal(t, resourceID, request.ResourceID)
	assert.Equal(t, agentType, request.AgentType)
	assert.True(t, request.APIKeyPresent)
	assert.Equal(t, remotePersistenceContentType, request.Accept)
	assert.True(t, request.AgentVersionPresent)
	assert.True(t, strings.HasPrefix(request.UserAgent, "datadog-agent/"))
}

func assertIssueState(t testing.TB, issue *healthplatform.Issue, issueID string, state healthplatform.IssueState) {
	t.Helper()
	require.NotNil(t, issue)
	assert.Equal(t, issueID, issue.GetId())
	assert.Equal(t, remotePersistenceIssueName, issue.GetIssueName())
	assert.Equal(t, remotePersistenceIssueType, issue.GetIssueType())
	require.NotNil(t, issue.GetPersistedIssue())
	assert.Equal(t, state, issue.GetPersistedIssue().GetState())
}

func findRemoteBackendRequest(requests []remoteBackendRequest, resourceID, agentType string) (remoteBackendRequest, bool) {
	for _, request := range requests {
		if request.ResourceID == resourceID && request.AgentType == agentType {
			return request, true
		}
	}
	return remoteBackendRequest{}, false
}

func filterRemoteBackendRequests(requests []remoteBackendRequest, agentType string) []remoteBackendRequest {
	var filtered []remoteBackendRequest
	for _, request := range requests {
		if request.AgentType == agentType {
			filtered = append(filtered, request)
		}
	}
	return filtered
}

func countRemoteBackendRequests(requests []remoteBackendRequest, agentType string) int {
	return len(filterRemoteBackendRequests(requests, agentType))
}

func findIssue(payloads []*aggregator.AgentHealthPayload, resourceID, issueID string, state healthplatform.IssueState) *healthplatform.Issue {
	for _, payload := range payloads {
		if payload == nil || payload.HealthReport == nil || payload.Host == nil || payload.Host.ResourceId != resourceID {
			continue
		}
		issue := payload.Issues[issueID]
		if issue != nil && issue.GetPersistedIssue().GetState() == state {
			return issue
		}
	}
	return nil
}

func findIssueByPrefix(payloads []*aggregator.AgentHealthPayload, resourceID, prefix, excludedID string, state healthplatform.IssueState) *healthplatform.Issue {
	for _, payload := range payloads {
		if payload == nil || payload.HealthReport == nil || payload.Host == nil || payload.Host.ResourceId != resourceID {
			continue
		}
		for issueID, issue := range payload.Issues {
			if issueID != excludedID && strings.HasPrefix(issueID, prefix) && issue.GetPersistedIssue().GetState() == state {
				return issue
			}
		}
	}
	return nil
}

func countIssueReports(payloads []*aggregator.AgentHealthPayload, resourceID, issueID string, state healthplatform.IssueState) int {
	count := 0
	for _, payload := range payloads {
		if findIssue([]*aggregator.AgentHealthPayload{payload}, resourceID, issueID, state) != nil {
			count++
		}
	}
	return count
}

func assertNoResolvedIssue(t testing.TB, payloads []*aggregator.AgentHealthPayload, resourceID, issueID string) {
	t.Helper()
	assert.Nil(t, findIssue(payloads, resourceID, issueID, healthplatform.IssueState_ISSUE_STATE_RESOLVED), "continuing issue was incorrectly resolved")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/version"
)

/*
Client to query the Datadog Cluster Agent (DCA) API.
*/

const (
	authorizationHeaderKey = "Authorization"
	// RealIPHeader refers to the cluster level check runner ip passed in the request headers
	RealIPHeader          = "X-Real-Ip"
	languageDetectionPath = "api/v1/languagedetection"
)

var globalClusterAgentClient *DCAClient

type metadataNames []string

// DCAClientInterface  is required to query the API of Datadog cluster agent
type DCAClientInterface interface {
	Version() version.Version
	ClusterAgentAPIEndpoint() string

	GetVersion() (version.Version, error)
	GetNodeLabels(nodeName string) (map[string]string, error)
	GetNodeAnnotations(nodeName string) (map[string]string, error)
	GetNamespaceLabels(nsName string) (map[string]string, error)
	GetPodsMetadataForNode(nodeName string) (apiv1.NamespacesPodsStringsSet, error)
	GetKubernetesMetadataNames(nodeName, ns, podName string) ([]string, error)
	GetCFAppsMetadataForNode(nodename string) (map[string][]string, error)

	PostClusterCheckStatus(ctx context.Context, nodeName string, status types.NodeStatus) (types.StatusResponse, error)
	GetClusterCheckConfigs(ctx context.Context, nodeName string) (types.ConfigResponse, error)
	GetEndpointsCheckConfigs(ctx context.Context, nodeName string) (types.ConfigResponse, error)
	GetKubernetesClusterID() (string, error)

	PostLanguageMetadata(ctx context.Context, data *pbgo.ParentLanguageAnnotationRequest) error
}

// DCAClient is required to query the API of Datadog cluster agent
type DCAClient struct {
	// used to setup the DCAClient
	initRetry retry.Retrier

	clusterAgentAPIEndpoint       string // ${SCHEME}://${clusterAgentHost}:${PORT}
	clusterAgentAPIRequestHeaders http.Header

	clusterAgentClientLock sync.RWMutex
	clusterAgentVersion    version.Version // Version of the cluster-agent we're connected to
	clusterAgentAPIClient  *http.Client
	leaderClient           *leaderClient
}

// resetGlobalClusterAgentClient is a helper to remove the current DCAClient global
// It is ONLY to be used for tests
func resetGlobalClusterAgentClient() {
	panic("not called")
}

// GetClusterAgentClient returns or init the DCAClient
func GetClusterAgentClient() (DCAClientInterface, error) {
	panic("not called")
}

func (c *DCAClient) init() error {
	panic("not called")
}

func (c *DCAClient) startReconnectHandler(reconnectPeriod time.Duration) {
	panic("not called")
}

func (c *DCAClient) initHTTPClient() error {
	panic("not called")
}

func (c *DCAClient) initLeaderClient() {
	panic("not called")
}

// GetClusterAgentEndpoint provides a validated https endpoint from configuration keys in datadog.yaml:
// 1st. configuration key "cluster_agent.url" (or the DD_CLUSTER_AGENT_URL environment variable),
//
//	add the https prefix if the scheme isn't specified
//
// 2nd. environment variables associated with "cluster_agent.kubernetes_service_name"
//
//	${dcaServiceName}_SERVICE_HOST and ${dcaServiceName}_SERVICE_PORT
func GetClusterAgentEndpoint() (string, error) {
	panic("not called")
}

// Version returns ClusterAgentVersion already stored in the DCAClient
func (c *DCAClient) Version() version.Version {
	panic("not called")
}

// ClusterAgentAPIEndpoint returns the Agent API Endpoint URL as a string
func (c *DCAClient) ClusterAgentAPIEndpoint() string {
	panic("not called")
}

// TODO: remove when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) buildURL(useLeaderClient bool, path string) string {
	panic("not called")
}

// TODO: remove when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) httpClient(useLeaderClient bool) *http.Client {
	panic("not called")
}

// TODO: remove the client parameter when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) doQuery(ctx context.Context, path, method string, body io.Reader, readResponseBody, useLeaderClient bool) ([]byte, error) {
	panic("not called")
}

func (c *DCAClient) doJSONQuery(ctx context.Context, path, method string, body io.Reader, obj interface{}, useLeaderClient bool) error {
	panic("not called")
}

// TODO: remove when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) doJSONQueryToLeader(ctx context.Context, path, method string, body io.Reader, obj interface{}) error {
	panic("not called")
}

// GetVersion fetches the version of the Cluster Agent. Used in the agent status command.
func (c *DCAClient) GetVersion() (version.Version, error) {
	panic("not called")
}

// GetNodeLabels returns the node labels from the Cluster Agent.
func (c *DCAClient) GetNodeLabels(nodeName string) (map[string]string, error) {
	panic("not called")
}

// GetNamespaceLabels returns the namespace labels from the Cluster Agent.
func (c *DCAClient) GetNamespaceLabels(nsName string) (map[string]string, error) {
	panic("not called")
}

// GetNodeAnnotations returns the node annotations from the Cluster Agent.
func (c *DCAClient) GetNodeAnnotations(nodeName string) (map[string]string, error) {
	panic("not called")
}

// GetCFAppsMetadataForNode returns the CF application tags from the Cluster Agent.
func (c *DCAClient) GetCFAppsMetadataForNode(nodename string) (map[string][]string, error) {
	panic("not called")
}

// GetPodsMetadataForNode queries the datadog cluster agent to get nodeName registered
// Kubernetes pods metadata.
func (c *DCAClient) GetPodsMetadataForNode(nodeName string) (apiv1.NamespacesPodsStringsSet, error) {
	panic("not called")
}

// GetKubernetesMetadataNames queries the datadog cluster agent to get nodeName/podName registered
// Kubernetes metadata.
func (c *DCAClient) GetKubernetesMetadataNames(nodeName, ns, podName string) ([]string, error) {
	panic("not called")
}

// GetKubernetesClusterID queries the datadog cluster agent to get the Kubernetes cluster ID
// Prefer calling clustername.GetClusterID which has a cached response
func (c *DCAClient) GetKubernetesClusterID() (string, error) {
	panic("not called")
}

// PostLanguageMetadata is called by the core-agent's language detection client
func (c *DCAClient) PostLanguageMetadata(ctx context.Context, data *pbgo.ParentLanguageAnnotationRequest) error {
	panic("not called")
}

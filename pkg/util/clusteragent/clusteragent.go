// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/DataDog/datadog-agent/pkg/version"
	"google.golang.org/protobuf/proto"
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

// LanguageDetectionClient defines the method to send a message to the Cluster-Agent
type LanguageDetectionClient interface {
	PostLanguageMetadata(ctx context.Context, data *pbgo.ParentLanguageAnnotationRequest) error
}

// DCAClientInterface  is required to query the API of Datadog cluster agent
type DCAClientInterface interface {
	LanguageDetectionClient
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
	globalClusterAgentClient = nil
}

// GetClusterAgentClient returns or init the DCAClient
func GetClusterAgentClient() (DCAClientInterface, error) {
	if globalClusterAgentClient == nil {
		globalClusterAgentClient = &DCAClient{}
		globalClusterAgentClient.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "clusterAgentClient",
			AttemptMethod:     globalClusterAgentClient.init,
			Strategy:          retry.Backoff,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
	}
	if err := globalClusterAgentClient.initRetry.TriggerRetry(); err != nil {
		log.Debugf("Cluster Agent init error: %v", err)
		return nil, err
	}
	return globalClusterAgentClient, nil
}

func (c *DCAClient) init() error {
	var err error

	c.clusterAgentAPIEndpoint, err = GetClusterAgentEndpoint()
	if err != nil {
		return err
	}

	authToken, err := security.GetClusterAgentAuthToken()
	if err != nil {
		return err
	}

	c.clusterAgentAPIRequestHeaders = http.Header{}
	c.clusterAgentAPIRequestHeaders.Set(authorizationHeaderKey, fmt.Sprintf("Bearer %s", authToken))
	podIP := config.Datadog.GetString("clc_runner_host")
	c.clusterAgentAPIRequestHeaders.Set(RealIPHeader, podIP)

	if err := c.initHTTPClient(); err != nil {
		return err
	}

	// Run DCA connection refresh
	c.startReconnectHandler(time.Duration(config.Datadog.GetInt64("cluster_agent.client_reconnect_period_seconds")) * time.Second)

	log.Infof("Successfully connected to the Datadog Cluster Agent %s", c.clusterAgentVersion.String())
	return nil
}

func (c *DCAClient) startReconnectHandler(reconnectPeriod time.Duration) {
	if reconnectPeriod <= 0 {
		return
	}

	t := time.NewTicker(reconnectPeriod)
	go func() {
		for {
			<-t.C
			err := c.initHTTPClient()
			if err != nil {
				log.Infof("Failed to re-create HTTP Connection, err: %v", err)
			}
		}
	}()
}

func (c *DCAClient) initHTTPClient() error {
	var err error
	// Copy of http.DefaulTransport with adapted settings
	clusterAgentAPIClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   1 * time.Second,
				KeepAlive: 20 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout:   5 * time.Second,
			MaxConnsPerHost:       1,
			MaxIdleConnsPerHost:   1,
			IdleConnTimeout:       60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 3 * time.Second,
		},
		Timeout: 10 * time.Second,
	}

	// We need to have a client to perform `GetVersion`, only happens during the first call
	if c.clusterAgentAPIClient == nil {
		c.clusterAgentAPIClient = clusterAgentAPIClient
	}

	// Validate the cluster-agent client by checking the version
	clusterAgentVersion, err := c.GetVersion()
	if err != nil {
		return err
	}

	c.clusterAgentClientLock.Lock()
	defer c.clusterAgentClientLock.Unlock()
	c.clusterAgentAPIClient = clusterAgentAPIClient
	c.clusterAgentVersion = clusterAgentVersion

	// Before DCA 1.21, we cannot rely on DCA follower forwarding, creating a leaderClient in this case
	// TODO: Remove when we drop compatibility
	if c.clusterAgentVersion.Major == 1 && c.clusterAgentVersion.Minor < 21 {
		log.Warnf("You're using an older Cluster Agent version. Newer Agent versions work best with Cluster Agent >= 1.21")
		c.initLeaderClient()
	}

	return nil
}

func (c *DCAClient) initLeaderClient() {
	c.leaderClient = newLeaderClient(c.clusterAgentAPIClient, c.clusterAgentAPIEndpoint)
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
	const configDcaURL = "cluster_agent.url"
	const configDcaSvcName = "cluster_agent.kubernetes_service_name"

	dcaURL := config.Datadog.GetString(configDcaURL)
	if dcaURL != "" {
		if strings.HasPrefix(dcaURL, "http://") {
			return "", fmt.Errorf("cannot get cluster agent endpoint, not a https scheme: %s", dcaURL)
		}
		if !strings.Contains(dcaURL, "://") {
			log.Tracef("Adding https scheme to %s: https://%s", dcaURL, dcaURL)
			dcaURL = fmt.Sprintf("https://%s", dcaURL)
		}
		u, err := url.Parse(dcaURL)
		if err != nil {
			return "", err
		}
		if u.Scheme != "https" {
			return "", fmt.Errorf("cannot get cluster agent endpoint, not a https scheme: %s", u.Scheme)
		}
		log.Debugf("Connecting to the configured URL for the Datadog Cluster Agent: %s", dcaURL)
		return u.String(), nil
	}

	// Construct the URL with the Kubernetes service environment variables
	// *_SERVICE_HOST and *_SERVICE_PORT
	dcaSvc := config.Datadog.GetString(configDcaSvcName)
	log.Debugf("Identified service for the Datadog Cluster Agent: %s", dcaSvc)
	if dcaSvc == "" {
		return "", fmt.Errorf("cannot get a cluster agent endpoint, both %s and %s are empty", configDcaURL, configDcaSvcName)
	}

	dcaSvc = strings.ToUpper(dcaSvc)
	dcaSvc = strings.Replace(dcaSvc, "-", "_", -1) // Kubernetes replaces "-" with "_" in the service names injected in the env var.

	// host
	dcaSvcHostEnv := fmt.Sprintf("%s_SERVICE_HOST", dcaSvc)
	dcaSvcHost := os.Getenv(dcaSvcHostEnv)
	if dcaSvcHost == "" {
		return "", fmt.Errorf("cannot get a cluster agent endpoint for kubernetes service %s, env %s is empty", dcaSvc, dcaSvcHostEnv)
	}

	// port
	dcaSvcPort := os.Getenv(fmt.Sprintf("%s_SERVICE_PORT", dcaSvc))
	if dcaSvcPort == "" {
		return "", fmt.Errorf("cannot get a cluster agent endpoint for kubernetes service %s, env %s is empty", dcaSvc, dcaSvcPort)
	}

	// validate the URL
	dcaURL = fmt.Sprintf("https://%s:%s", dcaSvcHost, dcaSvcPort)
	u, err := url.Parse(dcaURL)
	if err != nil {
		return "", err
	}

	return u.String(), nil
}

// Version returns ClusterAgentVersion already stored in the DCAClient
func (c *DCAClient) Version() version.Version {
	c.clusterAgentClientLock.RLock()
	defer c.clusterAgentClientLock.RUnlock()

	return c.clusterAgentVersion
}

// ClusterAgentAPIEndpoint returns the Agent API Endpoint URL as a string
func (c *DCAClient) ClusterAgentAPIEndpoint() string {
	return c.clusterAgentAPIEndpoint
}

// TODO: remove when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) buildURL(useLeaderClient bool, path string) string {
	if useLeaderClient && c.leaderClient != nil {
		return c.leaderClient.buildURL(path)
	}

	return c.clusterAgentAPIEndpoint + "/" + path
}

// TODO: remove when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) httpClient(useLeaderClient bool) *http.Client {
	c.clusterAgentClientLock.RLock()
	defer c.clusterAgentClientLock.RUnlock()

	if useLeaderClient && c.leaderClient != nil {
		return &c.leaderClient.Client
	}

	return c.clusterAgentAPIClient
}

// TODO: remove the client parameter when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) doQuery(ctx context.Context, path, method string, body io.Reader, readResponseBody, useLeaderClient bool) ([]byte, error) {
	url := c.buildURL(useLeaderClient, path)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("unable to build request during query to: %s, err: %w", url, err)
	}
	req.Header = c.clusterAgentAPIRequestHeaders

	client := c.httpClient(useLeaderClient)
	resp, err := client.Do(req)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			err = errors.NewTimeoutError(url, err)
		}

		return nil, errors.NewRemoteServiceError(url, err.Error())
	}
	defer resp.Body.Close()

	if readResponseBody && resp.StatusCode == http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.NewRemoteServiceError(url, err.Error())
		}

		return respBody, nil
	}

	// Make sure we read always body, required to re-use HTTP Connections
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.NewRemoteServiceError(url, resp.Status)
	}
	return nil, nil
}

func (c *DCAClient) doJSONQuery(ctx context.Context, path, method string, body io.Reader, obj interface{}, useLeaderClient bool) error {
	respBody, err := c.doQuery(ctx, path, method, body, true, useLeaderClient)
	if err != nil {
		return err
	}

	err = json.Unmarshal(respBody, obj)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON from URL: %s, err: %w, raw message: %q", path, err, respBody)
	}

	return nil
}

// TODO: remove when we drop compatibility with older Agents, see end of `init()`
func (c *DCAClient) doJSONQueryToLeader(ctx context.Context, path, method string, body io.Reader, obj interface{}) error {
	if c.leaderClient == nil {
		return c.doJSONQuery(ctx, path, method, body, obj, false)
	}

	willRetry := c.leaderClient.hasLeader()
	err := c.doJSONQuery(ctx, path, method, body, obj, true)
	if err != nil && willRetry {
		log.Debugf("Got error on leader, retrying via the service: %v", err)
		c.leaderClient.resetURL()
		err = c.doJSONQuery(ctx, path, method, body, obj, true)
	}

	return err
}

// GetVersion fetches the version of the Cluster Agent. Used in the agent status command.
func (c *DCAClient) GetVersion() (version.Version, error) {
	var version version.Version
	err := c.doJSONQuery(context.TODO(), "version", "GET", nil, &version, false)
	return version, err
}

// GetNodeLabels returns the node labels from the Cluster Agent.
func (c *DCAClient) GetNodeLabels(nodeName string) (map[string]string, error) {
	var result map[string]string
	err := c.doJSONQuery(context.TODO(), "api/v1/tags/node/"+nodeName, "GET", nil, &result, false)
	return result, err
}

// GetNamespaceLabels returns the namespace labels from the Cluster Agent.
func (c *DCAClient) GetNamespaceLabels(nsName string) (map[string]string, error) {
	var result map[string]string
	err := c.doJSONQuery(context.TODO(), "api/v1/tags/namespace/"+nsName, "GET", nil, &result, false)
	return result, err
}

// GetNodeAnnotations returns the node annotations from the Cluster Agent.
func (c *DCAClient) GetNodeAnnotations(nodeName string) (map[string]string, error) {
	var result map[string]string
	err := c.doJSONQuery(context.TODO(), "api/v1/annotations/node/"+nodeName, "GET", nil, &result, false)
	return result, err
}

// GetCFAppsMetadataForNode returns the CF application tags from the Cluster Agent.
func (c *DCAClient) GetCFAppsMetadataForNode(nodename string) (map[string][]string, error) {
	var result map[string][]string
	err := c.doJSONQuery(context.TODO(), "api/v1/tags/cf/apps/"+nodename, "GET", nil, &result, false)
	return result, err
}

// GetPodsMetadataForNode queries the datadog cluster agent to get nodeName registered
// Kubernetes pods metadata.
func (c *DCAClient) GetPodsMetadataForNode(nodeName string) (apiv1.NamespacesPodsStringsSet, error) {
	/* https://host:port/api/v1/tags/pod/{nodeName}
	response example:
	{
		"Nodes": {
			"node1": {
				"services": {
					"default": {
						"datadog-monitoring-cluster-agent-58f45b9b44-pkxrv": {
							"datadog-monitoring-cluster-agent": {},
							"datadog-monitoring-cluster-agent-metrics-api": {}
						}
					},
					"kube-system": {
						"kube-dns-6b98c9c9bf-ts7gc": {
							"kube-dns": {}
						}
					}
				}
			}
		}
	}
	*/
	metadataPodPayload := apiv1.NewMetadataResponse()
	err := c.doJSONQuery(context.TODO(), "api/v1/tags/pod/"+nodeName, "GET", nil, metadataPodPayload, false)
	if err != nil {
		return nil, err
	}
	if _, ok := metadataPodPayload.Nodes[nodeName]; !ok {
		return nil, fmt.Errorf("cluster agent didn't return pods metadata for node: %s", nodeName)
	}
	return metadataPodPayload.Nodes[nodeName].Services, nil
}

// GetKubernetesMetadataNames queries the datadog cluster agent to get nodeName/podName registered
// Kubernetes metadata.
func (c *DCAClient) GetKubernetesMetadataNames(nodeName, ns, podName string) ([]string, error) {
	var metadataNames metadataNames
	err := c.doJSONQuery(context.TODO(), fmt.Sprintf("api/v1/tags/pod/%s/%s/%s", nodeName, ns, podName), "GET", nil, &metadataNames, false)
	if err != nil {
		return nil, err
	}
	return metadataNames, nil
}

// GetKubernetesClusterID queries the datadog cluster agent to get the Kubernetes cluster ID
// Prefer calling clustername.GetClusterID which has a cached response
func (c *DCAClient) GetKubernetesClusterID() (string, error) {
	var clusterID string
	err := c.doJSONQuery(context.TODO(), "api/v1/cluster/id", "GET", nil, &clusterID, false)
	if err != nil {
		return "", err
	}
	return clusterID, nil
}

// PostLanguageMetadata is called by the core-agent's language detection client
func (c *DCAClient) PostLanguageMetadata(ctx context.Context, data *pbgo.ParentLanguageAnnotationRequest) error {
	queryBody, err := proto.Marshal(data)
	if err != nil {
		return err
	}

	// https://host:port/api/v1/languagedetection}
	err = c.doJSONQueryToLeader(ctx, languageDetectionPath, "POST", bytes.NewBuffer(queryBody), nil)
	return err
}

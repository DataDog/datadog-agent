// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

/*
Client to query the Datadog Cluster Agent (DCA) API.
*/

const (
	authorizationHeaderKey = "Authorization"
)

var globalClusterAgentClient *DCAClient

type metadataNames []string

// DCAClient is required to query the API of Datadog cluster agent
type DCAClient struct {
	// used to setup the DCAClient
	initRetry retry.Retrier

	clusterAgentAPIEndpoint       string // ${SCHEME}://${clusterAgentHost}:${PORT}
	clusterAgentAPIClient         *http.Client
	clusterAgentAPIRequestHeaders *http.Header
}

// GetClusterAgentClient returns or init the DCAClient
func GetClusterAgentClient() (*DCAClient, error) {
	if globalClusterAgentClient == nil {
		globalClusterAgentClient = &DCAClient{}
		globalClusterAgentClient.initRetry.SetupRetrier(&retry.Config{
			Name:          "clusterAgentClient",
			AttemptMethod: globalClusterAgentClient.init,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	if err := globalClusterAgentClient.initRetry.TriggerRetry(); err != nil {
		log.Debugf("Init error: %s", err)
		return nil, err
	}
	return globalClusterAgentClient, nil
}

func (c *DCAClient) init() error {
	var err error

	c.clusterAgentAPIEndpoint, err = getClusterAgentEndpoint()
	if err != nil {
		return err
	}

	authToken, err := security.GetClusterAgentAuthToken()
	if err != nil {
		return err
	}

	c.clusterAgentAPIRequestHeaders = &http.Header{}
	c.clusterAgentAPIRequestHeaders.Set(authorizationHeaderKey, fmt.Sprintf("Bearer %s", authToken))

	// TODO remove insecure
	c.clusterAgentAPIClient = util.GetClient(false)
	c.clusterAgentAPIClient.Timeout = 2 * time.Second

	return nil
}

// getClusterAgentEndpoint provides a validated https endpoint from configuration keys in datadog.yaml:
// 1st. configuration key "cluster_agent_url", add the https prefix if the scheme isn't specified
// 2nd. environment variables associated with "cluster_agent_kubernetes_service_name"
//      ${dcaServiceName}_SERVICE_HOST and ${dcaServiceName}_SERVICE_PORT
func getClusterAgentEndpoint() (string, error) {
	const configDcaURL = "cluster_agent.url"
	const configDcaSvcName = "cluster_agent.kubernetes_service_name"

	dcaURL := config.Datadog.GetString(configDcaURL)
	if dcaURL != "" {
		if strings.HasPrefix(dcaURL, "http://") {
			return "", fmt.Errorf("cannot get cluster agent endpoint, not a https scheme: %s", dcaURL)
		}
		if strings.Contains(dcaURL, "://") == false {
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
		return "", fmt.Errorf("cannot get a cluster agent endpoint, both %q and %q are empty", configDcaURL, configDcaSvcName)
	}

	dcaSvc = strings.ToUpper(dcaSvc)

	// host
	dcaSvcHostEnv := fmt.Sprintf("%s_SERVICE_HOST", dcaSvc)
	dcaSvcHost := os.Getenv(dcaSvcHostEnv)
	if dcaSvcHost == "" {
		return "", fmt.Errorf("cannot get a cluster agent endpoint for kubernetes service %q, env %q is empty", dcaSvc, dcaSvcHostEnv)
	}

	// port
	dcaSvcPort := os.Getenv(fmt.Sprintf("%s_SERVICE_PORT", dcaSvc))
	if dcaSvcPort == "" {
		return "", fmt.Errorf("cannot get a cluster agent endpoint for kubernetes service %q, env %q is empty", dcaSvc, dcaSvcPort)
	}

	// validate the URL
	dcaURL = fmt.Sprintf("https://%s:%s", dcaSvcHost, dcaSvcPort)
	u, err := url.Parse(dcaURL)
	if err != nil {
		return "", err
	}

	return u.String(), nil
}

// GetKubernetesMetadataNames queries the datadog cluster agent to get nodeName/podName registered
// Kubernetes metadata.
func (c *DCAClient) GetKubernetesMetadataNames(nodeName, podName string) ([]string, error) {
	const dcaMetadataPath = "api/v1/metadata"
	var metadataNames metadataNames
	var err error

	if c == nil {
		return nil, fmt.Errorf("cluster agent's client is not properly initialized")
	}
	req := &http.Request{
		Header: *c.clusterAgentAPIRequestHeaders,
	}
	// https://host:port /api/v1/metadata/ {nodeName}/ {pod-[0-9a-z]+}
	rawURL := fmt.Sprintf("%s/%s/%s/%s", c.clusterAgentAPIEndpoint, dcaMetadataPath, nodeName, podName)
	req.URL, err = url.Parse(rawURL)
	if err != nil {
		return metadataNames, err
	}

	resp, err := c.clusterAgentAPIClient.Do(req)
	if err != nil {
		return metadataNames, err
	}

	if resp.StatusCode != http.StatusOK {
		return metadataNames, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return metadataNames, err
	}
	err = json.Unmarshal(b, &metadataNames)
	if err != nil {
		return metadataNames, err
	}

	return metadataNames, nil
}

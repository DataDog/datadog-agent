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
	"path"
	"strings"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

/*
Client to query the Datadog Cluster Agent (DCA) API.
*/

const (
	authorizationHeaderKey        = "Authorization"
	clusterAgentAuthTokenMinLen   = 32
	clusterAgentAuthTokenFilename = "dca_auth_token"
)

var globalClusterAgentClient *DCAClient

type serviceNames []string

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
	err := globalClusterAgentClient.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("Init error: %s", err)
		return nil, err
	}
	return globalClusterAgentClient, nil
}

func validateAuthToken(authToken string) error {
	if len(authToken) < clusterAgentAuthTokenMinLen {
		return fmt.Errorf("cluster agent authentication token length must be greater than %d, curently: %d", clusterAgentAuthTokenMinLen, len(authToken))
	}
	return nil
}

// GetClusterAgentAuthToken load the authentication token from:
// 1srt. the configuration value of "cluster_agent_auth_token" in datadog.yaml
// 2nd. from the filesystem
// If using the token from the filesystem, the token file must be next to the datadog.yaml
// with the filename: dca_auth_token
func GetClusterAgentAuthToken() (string, error) {
	authToken := config.Datadog.GetString("cluster_agent.auth_token")
	if authToken != "" {
		return authToken, validateAuthToken(authToken)
	}

	// load the cluster agent auth token from filesystem
	tokenAbsPath := path.Join(config.FileUsedDir(), clusterAgentAuthTokenFilename)
	log.Debugf("empty cluster_agent_auth_token, loading from %s", tokenAbsPath)
	_, err := os.Stat(tokenAbsPath)
	if err != nil {
		return "", fmt.Errorf("empty cluster_agent_auth_token and cannot find %q: %s", tokenAbsPath, err)
	}
	b, err := ioutil.ReadFile(tokenAbsPath)
	if err != nil {
		return "", fmt.Errorf("empty cluster_agent_auth_token and cannot read %s: %s", tokenAbsPath, err)
	}
	log.Debugf("cluster_agent_auth_token loaded from %s", tokenAbsPath)

	authToken = string(b)
	return authToken, validateAuthToken(authToken)
}

func (c *DCAClient) init() error {
	var err error

	c.clusterAgentAPIEndpoint, err = getClusterAgentEndpoint()
	if err != nil {
		return err
	}

	authToken, err := GetClusterAgentAuthToken()
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
			log.Tracef("adding https scheme to %s: https://%s", dcaURL, dcaURL)
			dcaURL = fmt.Sprintf("https://%s", dcaURL)
		}
		u, err := url.Parse(dcaURL)
		if err != nil {
			return "", err
		}
		if u.Scheme != "https" {
			return "", fmt.Errorf("cannot get cluster agent endpoint, not a https scheme: %s", u.Scheme)
		}
		return u.String(), nil
	}

	// Construct the URL with the Kubernetes service environment variables
	// *_SERVICE_HOST and *_SERVICE_PORT
	dcaSvc := config.Datadog.GetString(configDcaSvcName)
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

// GetKubernetesServiceNames queries the datadog cluster agent to get nodeName/podName registered
// Kubernetes services.
func (c *DCAClient) GetKubernetesServiceNames(nodeName, podName string) ([]string, error) {
	const dcaMetadataPath = "api/v1/metadata"
	var serviceNames serviceNames
	var err error

	req := &http.Request{
		Header: *c.clusterAgentAPIRequestHeaders,
	}
	// https://host:port /api/v1/metadata/ {nodeName}/ {pod-[0-9a-z]+}
	rawURL := fmt.Sprintf("%s/%s/%s/%s", c.clusterAgentAPIEndpoint, dcaMetadataPath, nodeName, podName)
	req.URL, err = url.Parse(rawURL)
	if err != nil {
		return serviceNames, err
	}

	resp, err := c.clusterAgentAPIClient.Do(req)
	if err != nil {
		return serviceNames, err
	}

	if resp.StatusCode != http.StatusOK {
		return serviceNames, fmt.Errorf("unexpected status code from cluster agent: %d", resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return serviceNames, err
	}
	err = json.Unmarshal(b, &serviceNames)
	if err != nil {
		return serviceNames, err
	}

	return serviceNames, nil
}

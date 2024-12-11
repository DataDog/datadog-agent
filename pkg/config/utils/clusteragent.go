// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

	dcaURL := pkgconfigsetup.Datadog().GetString(configDcaURL)
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
	dcaSvc := pkgconfigsetup.Datadog().GetString(configDcaSvcName)
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

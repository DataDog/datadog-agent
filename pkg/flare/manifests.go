// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package flare

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	b64       = base64.StdEncoding
	magicGzip = []byte{0x1f, 0x8b, 0x08}
)

const (
	HELM_CHART_RELEASE_NAME       = "CHART_RELEASE_NAME"
	HELM_CHART_RELEASE_NAMESPACE  = "DD_KUBE_RESOURCES_NAMESPACE"
	HELM_AGENT_DAEMONSET          = "AGENT_DAEMONSET"
	HELM_CLUSTER_AGENT_DEPLOYMENT = "CLUSTER_AGENT_DEPLOYMENT"
)

// chartUserValues is defined to unmarshall JSON data decoded from a Helm cart release into accessible fields
type chartUserValues struct {
	// user-defined values overriding the chart defaults
	Config map[string]interface{} `json:"config,omitempty"`
}

// convertToYAMLBytes is a helper function to turn an object returned from `k8s.io/api/core/v1` into a readable YAML manifest
func convertToYAMLBytes(input any) ([]byte, error) {
	objJson, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("Unable to Marshal the object manifest: %w", err)
	}
	return yaml.JSONToYAML(objJson)
}

// Retrieve a DaemonSet YAML from the API server for a given name and namespace, and returns the associated YAML manifest into a a byte array.
// Its purpose is to retrieve the Datadog Agent DaemonSet manifest when building a Cluster Agent flare.
func getDaemonset(cl *apiserver.APIClient, name string, namespace string) ([]byte, error) {
	ds, err := cl.Cl.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Debugf("Can't retrieve DaemonSet %v from the API server: %s", name, err.Error())
		return nil, err
	}
	return convertToYAMLBytes(ds)
}

// getAgentDaemonSet retrieves the DaemonSet manifest of the Agent
func getAgentDaemonSet() ([]byte, error) {
	// The Agent DaemonSet name is based on the Helm chart template and added to the Cluster Agent as an environment variable
	var agentDaemonsetName string
	var releaseNamespace string
	var agentDaemonset []byte

	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, log.Errorf("Can't create client to query the API Server: %s", err)
	}
	agentDaemonsetName = os.Getenv(HELM_AGENT_DAEMONSET)
	releaseNamespace = os.Getenv(HELM_CHART_RELEASE_NAMESPACE)
	if agentDaemonsetName == "" || releaseNamespace == "" {
		return nil, log.Errorf("Can't collect the Agent Daemonset name and/or namespace from the environment variables %s and %v", HELM_AGENT_DAEMONSET, HELM_CHART_RELEASE_NAMESPACE)
	}
	agentDaemonset, err = getDaemonset(cl, agentDaemonsetName, releaseNamespace)
	if err != nil {
		return nil, log.Errorf("Error while collecting the Agent DaemonSet: %q", err)
	}
	return agentDaemonset, nil
}

// Retrieve a Deployment YAML from the API server for a given name and namespace, and returns the associated YAML manifest into a a byte array.
// Its purpose is to retrieve the Datadog Cluster Agent Deployment manifest when building a Cluster Agent flare.
func getDeployment(cl *apiserver.APIClient, name string, namespace string) ([]byte, error) {
	deploy, err := cl.Cl.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Debugf("Can't retrieve Deployment %v from the API server: %s", name, err.Error())
		return nil, err
	}
	return convertToYAMLBytes(deploy)
}

// getClusterAgentDeployment retrieves the Deployment manifest of the Cluster Agent
func getClusterAgentDeployment() ([]byte, error) {
	// The Cluster Agent Deployment name is based on the Helm chart template and added to the Cluster Agent as an environment variable
	var clusterAgentDeploymentName string
	var releaseNamespace string
	var clusterAgentDeployment []byte

	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, log.Errorf("Can't create client to query the API Server: %s", err)
	}
	clusterAgentDeploymentName = os.Getenv(HELM_CLUSTER_AGENT_DEPLOYMENT)
	releaseNamespace = os.Getenv(HELM_CHART_RELEASE_NAMESPACE)
	if clusterAgentDeploymentName == "" || releaseNamespace == "" {
		return nil, log.Errorf("Can't collect the Cluster Agent Deployment name and/or namespace from the environment variables %s and %v", HELM_CLUSTER_AGENT_DEPLOYMENT, HELM_CHART_RELEASE_NAMESPACE)
	}
	clusterAgentDeployment, err = getDeployment(cl, clusterAgentDeploymentName, releaseNamespace)
	if err != nil {
		return nil, log.Errorf("Error while collecting the Cluster Agent Deployment: %q", err)
	}
	return clusterAgentDeployment, nil
}

// getDeployedHelmConfigmap returns the configmap for a given release.
// Only a single release for a given name can be deployed at one time.
func getDeployedHelmConfigmap(cl *apiserver.APIClient, name string, namespace string) (*v1.ConfigMap, error) {
	var selector string

	selector = labels.Set{
		"owner":  "helm",
		"status": "deployed",
		"name":   name,
	}.AsSelector().String()
	configmapList, err := cl.Cl.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if len(configmapList.Items) != 1 {
		return nil, log.Errorf("%s configmaps found, but expected 1", fmt.Sprint(len(configmapList.Items)))
	}
	return &configmapList.Items[0], nil
}

// getDeployedHelmSecret returns the secret for a given release.
// Only a single release for a given name can be deployed at one time.
func getDeployedHelmSecret(cl *apiserver.APIClient, name string, namespace string) (*v1.Secret, error) {
	var selector string

	selector = labels.Set{
		"owner":  "helm",
		"status": "deployed",
		"name":   name,
	}.AsSelector().String()
	secretList, err := cl.Cl.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	if len(secretList.Items) != 1 {
		return nil, log.Errorf("%s secrets found, but expected 1", fmt.Sprint(len(secretList.Items)))
	}
	return &secretList.Items[0], nil
}

// decodeRelease decodes the bytes of data into a readable byte array.
// Data must contain a base64 encoded gzipped string of a valid release, otherwise nil is returned.
func decodeRelease(data string) ([]byte, error) {
	// base64 decode string
	b, err := b64.DecodeString(data)
	if err != nil {
		return nil, err
	}

	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if len(b) < 4 {
		// Avoid panic if b[0:3] cannot be accessed
		return nil, log.Errorf("The byte array is too short (expected at least 4 characters, got %s instead): it cannot contain a Helm release", fmt.Sprint(len(b)))
	}
	if bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}
	return b, nil
}

// decodeChartValuesFromRelease returns a byte array with the user values from an encoded Helm chart release
func decodeChartValuesFromRelease(encodedRelease string) ([]byte, error) {
	var userConfig chartUserValues

	decodedrelease, err := decodeRelease(encodedRelease)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(decodedrelease, &userConfig)
	if err != nil {
		log.Debugf("Unable to retrieve the config data: %s", err.Error())
		return nil, err
	}
	configjson, err := json.Marshal(userConfig)
	if err != nil {
		log.Debugf("Can't marshall user values into a proper JSON: %s", err.Error())
		return nil, err
	}
	return yaml.JSONToYAML(configjson)
}

// getHelmValues retrieves the user-defined values for the Datadog Helm chart
func getHelmValues() ([]byte, error) {
	var dataString string
	var helmUserValues []byte
	var releaseName string
	var releaseNamespace string

	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, log.Errorf("Can't create client to query the API Server: %s", err)
	}
	releaseName = os.Getenv(HELM_CHART_RELEASE_NAME)
	releaseNamespace = os.Getenv(HELM_CHART_RELEASE_NAMESPACE)
	if releaseName == "" || releaseNamespace == "" {
		return nil, log.Errorf("Can't collect the Datadog Helm chart release name and/or namespace from the environment variables %s and %v", HELM_CHART_RELEASE_NAME, HELM_CHART_RELEASE_NAMESPACE)
	}
	// Attempting to retrieve Helm chart data from secrets (default storage in Helm v3)
	secret, err := getDeployedHelmSecret(cl, releaseName, releaseNamespace)
	if err != nil {
		log.Warnf("Error while collecting the Helm chart values from secret: %v", err)
	} else {
		// Contrary to the Configmap, the secret data is a byte array, so the string function is necessary
		dataString = string(secret.Data["release"])
		helmUserValues, err = decodeChartValuesFromRelease(dataString)
		if err != nil {
			log.Warnf("Unable to decode release stored in secret: %v", err)
		} else {
			return helmUserValues, nil
		}
	}
	// The cluster Agent was unable to retrieve Helm chart data from secrets, attempting to retrieve them from Configmaps
	configmap, err := getDeployedHelmConfigmap(cl, releaseName, releaseNamespace)
	if err != nil {
		log.Warnf("Error while collecting the Helm chart values from configmap: %v", err)
	} else {
		dataString = configmap.Data["release"]
		helmUserValues, err = decodeChartValuesFromRelease(dataString)
		if err != nil {
			log.Warnf("Unable to decode release stored in configmap: %v", err)
		} else {
			return helmUserValues, nil
		}
	}
	return nil, fmt.Errorf("Unable to collect Helm values from secrets/configmaps")
}

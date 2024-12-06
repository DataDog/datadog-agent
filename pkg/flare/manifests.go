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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	b64       = base64.StdEncoding
	magicGzip = []byte{0x1f, 0x8b, 0x08}
)

const (
	// Name of the Helm chart release to install the Datadog Agent
	helmChartReleaseName = "CHART_RELEASE_NAME"
	// Name of the DatadogAgent custom resource to install the Datadog Agent
	datadogAgentCustomResourceName = "DATADOGAGENT_CR_NAME"
	// Namespace where the Datadog Agent and Cluster Agent are installed
	agentResourcesNamespace = "DD_KUBE_RESOURCES_NAMESPACE"
	// Environment variables to retrieve the DaemonSet and Deployment names of the Agent and Cluster Agent
	agentDaemonsetEnvV     = "AGENT_DAEMONSET"
	clusterAgentDeployEnvV = "CLUSTER_AGENT_DEPLOYMENT"
)

// chartUserValues is defined to unmarshall JSON data decoded from a Helm chart release into accessible fields
type chartUserValues struct {
	// user-defined values overriding the chart defaults
	Config map[string]interface{} `json:"config,omitempty"`
}

// convertToYAMLBytes is a helper function to turn an object returned from `k8s.io/api/core/v1` into a readable YAML manifest
func convertToYAMLBytes(input any) ([]byte, error) {
	objJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("Unable to Marshal the object manifest: %w", err)
	}
	return yaml.JSONToYAML(objJSON)
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
	// The Agent DaemonSet name is based on the Helm chart template/DatadogAgent custom resource and added to the Cluster Agent as an environment variable
	var agentDaemonsetName string
	var releaseNamespace string
	var agentDaemonset []byte

	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, log.Errorf("Can't create client to query the API Server: %s", err)
	}
	agentDaemonsetName = os.Getenv(agentDaemonsetEnvV)
	releaseNamespace = os.Getenv(agentResourcesNamespace)
	if agentDaemonsetName == "" || releaseNamespace == "" {
		return nil, log.Errorf("Can't collect the Agent Daemonset name and/or namespace from the environment variables %s and %v", agentDaemonsetEnvV, agentResourcesNamespace)
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
	// The Cluster Agent Deployment name is based on the Helm chart template/DatadogAgent custom resource and added to the Cluster Agent as an environment variable
	var clusterAgentDeploymentName string
	var releaseNamespace string
	var clusterAgentDeployment []byte

	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, log.Errorf("Can't create client to query the API Server: %s", err)
	}
	clusterAgentDeploymentName = os.Getenv(clusterAgentDeployEnvV)
	releaseNamespace = os.Getenv(agentResourcesNamespace)
	if clusterAgentDeploymentName == "" || releaseNamespace == "" {
		return nil, log.Errorf("Can't collect the Cluster Agent Deployment name and/or namespace from the environment variables %s and %v", clusterAgentDeployEnvV, agentResourcesNamespace)
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
	selector := labels.Set{
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
	selector := labels.Set{
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
	releaseName = os.Getenv(helmChartReleaseName)
	releaseNamespace = os.Getenv(agentResourcesNamespace)
	if releaseName == "" || releaseNamespace == "" {
		return nil, log.Errorf("Can't collect the Datadog Helm chart release name and/or namespace from the environment variables %s and %v", helmChartReleaseName, agentResourcesNamespace)
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

// getDatadogAgentManifest retrieves the user-defined manifest for the Datadog Agent resource (managed by the Operator)
func getDatadogAgentManifest() ([]byte, error) {
	cl, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, log.Errorf("Can't create client to query the API Server: %s", err)
	}
	ddaName := os.Getenv(datadogAgentCustomResourceName)
	releaseNamespace := os.Getenv(agentResourcesNamespace)
	if ddaName == "" || releaseNamespace == "" {
		return nil, log.Errorf("Can't collect the Datadog Agent custom resource name and/or namespace from the environment variables %s and %s", datadogAgentCustomResourceName, agentResourcesNamespace)
	}

	// Retrieving the Datadog Agent custom resource from the API server using a dynamic client
	ddaGroupVersionResource := schema.GroupVersionResource{
		Group:    "datadoghq.com",
		Version:  "v2alpha1",
		Resource: "datadogagents",
	}
	dda, err := cl.DynamicCl.Resource(ddaGroupVersionResource).Namespace(releaseNamespace).Get(context.TODO(), ddaName, metav1.GetOptions{})

	if err != nil {
		return nil, log.Errorf("Can't retrieve the Datadog Agent custom resource %v from the API server: %s", ddaName, err.Error())
	}

	// Converting the custom resource into a readable YAML manifest
	return convertToYAMLBytes(dda.Object)
}

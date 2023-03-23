// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

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

// Retrieve a DaemonSet YAML from the API server for a given name and namespace, and returns the associated YAML manifest into a a byte array.
// Its purpose is to retrieve the Datadog Agent DaemonSet manifest when building a Cluster Agent flare.
func GetDaemonset(cl *apiserver.APIClient, name string, namespace string) ([]byte, error) {
	var b bytes.Buffer

	ds, err := cl.Cl.AppsV1().DaemonSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Debugf("Can't retrieve DaemonSet %v from the API server: %s", name, err.Error())
		return nil, err
	}

	dsjson, err := json.Marshal(ds)
	ddyaml, err := yaml.JSONToYAML(dsjson)
	fmt.Fprintln(&b, string(ddyaml))
	return b.Bytes(), nil
}

// Retrieve a Deployment YAML from the API server for a given name and namespace, and returns the associated YAML manifest into a a byte array.
// Its purpose is to retrieve the Datadog Cluster Agent Deployment manifest when building a Cluster Agent flare.
func GetDeployment(cl *apiserver.APIClient, name string, namespace string) ([]byte, error) {
	var b bytes.Buffer

	deploy, err := cl.Cl.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		log.Debugf("Can't retrieve Deployment %v from the API server: %s", name, err.Error())
		return nil, err
	}

	deployjson, err := json.Marshal(deploy)
	deployyaml, err := yaml.JSONToYAML(deployjson)
	fmt.Fprintln(&b, string(deployyaml))
	return b.Bytes(), nil
}

// getDeployedHelmConfigmap returns the configmap for a given release.
// Only a single release for a given name can deployed at one time.
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
		return nil, log.Errorf("%s configmaps found, but expected 1", fmt.Sprint((configmapList.Items)))
	}
	return &configmapList.Items[0], nil
}

// getDeployedHelmSecret returns the secret for a given release.
// Only a single release for a given name can deployed at one time.
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

// Retrieve Helm chart user values from the API server for a given name and namespace looking through Secrets or Configmaps
func GetDatadogHelmUserValues(cl *apiserver.APIClient, name string, namespace string, storage string) ([]byte, error) {
	switch storage {
	case "secret":
		secret, err := getDeployedHelmSecret(cl, name, namespace)
		if err != nil {
			return nil, err
		}
		b64EncodedGzipRelease := secret.Data["release"]
		// Contrary to the Configmap, the secret data is a byte array, so the string function is necessary
		b, err := decodeChartValuesFromRelease(string(b64EncodedGzipRelease))
		if err != nil {
			return nil, err
		}
		return b, nil
	case "configmap":
		configmap, err := getDeployedHelmConfigmap(cl, name, namespace)
		if err != nil {
			return nil, err
		}
		b64EncodedGzipRelease := configmap.Data["release"]
		b, err := decodeChartValuesFromRelease(b64EncodedGzipRelease)
		if err != nil {
			return nil, err
		}
		return b, nil
	default:
		return nil, log.Errorf("The requested storage %s is not supported", storage)
	}
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
	var b bytes.Buffer
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
	configyaml, err := yaml.JSONToYAML(configjson)
	if err != nil {
		log.Debugf("Can't convert user values to YAML: %s", err.Error())
		return nil, err
	}
	fmt.Fprintln(&b, string(configyaml))
	return b.Bytes(), nil
}

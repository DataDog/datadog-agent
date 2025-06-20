// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package clusteragentimpl implements the clusteragent metadata providers interface
package clusteragentimpl

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	releaseNamespace   = os.Getenv("DD_KUBE_RESOURCES_NAMESPACE")
	releaseTemplate    = "sh.helm.release.v1.%s.v"
	versionRegexp      = regexp.MustCompile(`\.v(\d+)$`)
	noHelmReleaseError = fmt.Errorf("no Helm release found in pod labels")
)

// HelmReleaseMinimal represents the minimal structure we care about
type HelmReleaseMinimal struct {
	Name   string                 `json:"name"`
	Config map[string]interface{} `json:"config"` // User-supplied values
}

// getLatestHelmRevision finds and returns the latest ConfigMap data for a Helm release.
func getLatestHelmRevision(ctx context.Context, clientset *kubernetes.Clientset, releaseName string) (int, error) {
	cmList, err := clientset.CoreV1().ConfigMaps(releaseNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("error listing ConfigMaps: %w", err)
	}

	maxVersion := 0
	for _, cm := range cmList.Items {
		if strings.HasPrefix(cm.Name, fmt.Sprintf(releaseTemplate, releaseName)) {
			match := versionRegexp.FindStringSubmatch(cm.Name)
			if len(match) != 2 {
				continue
			}

			rev, err := strconv.Atoi(match[1])
			if err != nil {
				continue
			}

			if rev > maxVersion {
				maxVersion = rev
			}
		}
	}

	if maxVersion == 0 {
		return 0, fmt.Errorf("no matching ConfigMaps found for release %q", releaseName)
	}
	return maxVersion, nil
}

// Retrieves the release name from the pod labels or returns a default value.
func getReleaseName(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
	// Get the pod
	pod, err := clientset.CoreV1().Pods(releaseNamespace).Get(context.TODO(), os.Getenv("DD_POD_NAME"), metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	// Get Helm release name from labels
	releaseName := pod.Labels["app.kubernetes.io/instance"]
	if releaseName != "" {
		return releaseName, nil
	}
	return "", noHelmReleaseError
}

func retrieveHelmValues(ctx context.Context) ([]byte, error) {
	restConfig, err := rest.InClusterConfig() // use kubeconfig for local dev
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config for metadata client: %w", err)
	}

	kubernetesClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata client: %w", err)
	}

	// Get the release name from the pod labels
	releaseName, err := getReleaseName(ctx, kubernetesClient)
	if err != nil {
		if err == noHelmReleaseError {
			return nil, nil // No Helm release found, return nil
		}
		return nil, fmt.Errorf("failed to get Helm release name: %w", err)
	}

	latestHelmRevision, err := getLatestHelmRevision(ctx, kubernetesClient, releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest Helm revision: %w", err)
	}

	// Get the configmap
	cm, err := kubernetesClient.CoreV1().ConfigMaps(releaseNamespace).Get(ctx, fmt.Sprintf("%s%d", fmt.Sprintf(releaseTemplate, releaseName), latestHelmRevision), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap for latest Helm revision: %w", err)
	}

	// Decode and decompress
	decoded, err := base64.StdEncoding.DecodeString(cm.Data["release"])
	if err != nil {
		log.Fatalf("Base64 decode error: %v", err)
	}

	gr, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		log.Fatalf("Gzip decompression error: %v", err)
	}
	defer gr.Close()

	var decompressed bytes.Buffer
	_, err = io.Copy(&decompressed, gr)
	if err != nil {
		log.Fatalf("GZIP read error: %v", err)
	}

	var release HelmReleaseMinimal
	if err := json.Unmarshal(decompressed.Bytes(), &release); err != nil {
		log.Fatalf("Unmarshal error: %v", err)
	}

	valuesYAML, err := yaml.Marshal(release.Config)
	if err != nil {
		log.Fatalf("YAML marshal error: %v", err)
	}

	return valuesYAML, nil
}

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
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

var (
	releaseNamespace = os.Getenv("DD_KUBE_RESOURCES_NAMESPACE")
	releaseTemplate  = "sh.helm.release.v1.%s.v"
	versionRegexp    = regexp.MustCompile(`\.v(\d+)$`)
	errNoHelmRelease = fmt.Errorf("no Helm release found in pod labels")
)

const (
	// helmValuesCacheTTL is the time-to-live for the cached Helm values (~90 minutes)
	helmValuesCacheTTL = 90 * time.Minute
)

// helmValuesCache holds the cached Helm values with timestamp
type helmValuesCache struct {
	mu        sync.RWMutex
	data      []byte
	timestamp time.Time
}

// global cache instance
var helmCache = &helmValuesCache{}

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
	podName, err := common.GetSelfPodName()
	if err != nil {
		return "", fmt.Errorf("could not fetch our self pod name: %w", err)
	}
	pod, err := clientset.CoreV1().Pods(releaseNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod: %w", err)
	}

	// Get Helm release name from labels
	releaseName := pod.Labels["app.kubernetes.io/instance"]
	if releaseName != "" {
		return releaseName, nil
	}
	return "", errNoHelmRelease
}

// getFromCache retrieves the cached Helm values if they exist and are not expired
func (c *helmValuesCache) getFromCache() ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.data == nil || time.Since(c.timestamp) > helmValuesCacheTTL {
		return nil, false
	}
	return c.data, true
}

// setCache stores the Helm values in the cache with the current timestamp
func (c *helmValuesCache) setCache(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = data
	c.timestamp = time.Now()
}

func retrieveHelmValues(ctx context.Context) ([]byte, error) {
	// Check if we have a valid cached value
	if cachedData, ok := helmCache.getFromCache(); ok {
		return cachedData, nil
	}

	// Cache miss or expired, fetch fresh data
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
		if err == errNoHelmRelease {
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
		return nil, fmt.Errorf("Base64 decode error: %v", err)
	}

	gr, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("Gzip decompression error: %v", err)
	}
	defer gr.Close()

	var decompressed bytes.Buffer
	_, err = io.Copy(&decompressed, gr)
	if err != nil {
		return nil, fmt.Errorf("GZIP read error: %v", err)
	}

	var release HelmReleaseMinimal
	if err := json.Unmarshal(decompressed.Bytes(), &release); err != nil {
		return nil, fmt.Errorf("Unmarshal error: %v", err)
	}

	valuesYAML, err := yaml.Marshal(release.Config)
	if err != nil {
		return nil, fmt.Errorf("YAML marshal error: %v", err)
	}

	// Store the result in cache before returning
	helmCache.setCache(valuesYAML)

	return valuesYAML, nil
}

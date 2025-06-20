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
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultHelmDriver = "configmap"
	releaseName       = "datadog-agent" // TODO: Retrieve this from pod labels
	releaseNamespace  = "default"       // TODO: Retrieve this from pod
)

var (
	releasePrefix = fmt.Sprintf("sh.helm.release.v1.%s.v", releaseName)
	versionRegexp = regexp.MustCompile(`\.v(\d+)$`)
)

// HelmReleaseMinimal represents the minimal structure we care about
type HelmReleaseMinimal struct {
	Name   string                 `json:"name"`
	Config map[string]interface{} `json:"config"` // User-supplied values
}

// getLatestHelmRevision finds and returns the latest ConfigMap data for a Helm release.
func getLatestHelmRevision(ctx context.Context, clientset *kubernetes.Clientset) (int, error) {
	cmList, err := clientset.CoreV1().ConfigMaps(releaseNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("error listing ConfigMaps: %w", err)
	}

	maxVersion := 0
	for _, cm := range cmList.Items {
		if strings.HasPrefix(cm.Name, releasePrefix) {
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

func retrieveHelmValues(ctx context.Context) ([]byte, error) {
	restConfig, err := rest.InClusterConfig() // use kubeconfig for local dev
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config for metadata client: %w", err)
	}

	kubernetesClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata client: %w", err)
	}

	latestHelmRevision, err := getLatestHelmRevision(ctx, kubernetesClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest Helm revision: %w", err)
	}

	// Get the configmap
	cm, err := kubernetesClient.CoreV1().ConfigMaps(releaseNamespace).Get(ctx, fmt.Sprintf("%s%d", releasePrefix, latestHelmRevision), metav1.GetOptions{})
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

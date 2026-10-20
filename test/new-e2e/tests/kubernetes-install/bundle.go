// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2026-present Datadog, Inc.

// Package kubernetesinstall tests instructions copied from the web-ui signup page.
package kubernetesinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

const keyMarker = "__K8S_INSTALL_API_KEY__"

type installCase struct {
	Method      string `json:"method"`
	ClusterName string `json:"clusterName"`
	Setup       string `json:"setup"`
	Config      string `json:"config"`
	Install     string `json:"install"`
}

type bundle struct {
	SchemaVersion int           `json:"schemaVersion"`
	Platform      string        `json:"platform"`
	SourceSHA     string        `json:"sourceSHA"`
	StaticHash    string        `json:"staticHash"`
	RunID         string        `json:"runID"`
	APIKeySHA256  string        `json:"apiKeySHA256"`
	Cases         []installCase `json:"cases"`
}

// Only reviewed commands may run on the CI host. Changes to the UI command
// grammar require review here; never pass the copied text to a shell.
var allowedCommands = map[string]bool{
	"helm repo add datadog https://helm.datadoghq.com": true,
	"helm repo update": true,
	"helm install datadog-operator datadog/datadog-operator --namespace datadog --create-namespace":    true,
	"kubectl create secret generic datadog-secret --from-literal api-key=" + keyMarker + " -n datadog": true,
	"kubectl -n datadog create secret generic datadog-secret --from-literal api-key=" + keyMarker:      true,
	"kubectl create namespace datadog":                                                      true,
	"kubectl apply -f datadog-agent.yaml":                                                   true,
	"helm install datadog-agent -f datadog-values.yaml datadog/datadog --namespace datadog": true,
}

func commands(text string) ([][]string, error) {
	var result [][]string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		if !allowedCommands[line] {
			return nil, errors.New("unreviewed installation command; refusing to execute or print it")
		}
		result = append(result, strings.Fields(line))
	}
	return result, nil
}

func parseBundle(data []byte, sourceSHA string) (bundle, error) {
	var b bundle
	if len(data) > 1<<20 {
		return b, errors.New("instruction bundle exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&b); err != nil {
		return b, errors.New("invalid instruction bundle")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return b, errors.New("trailing content in instruction bundle")
	}
	if b.Platform != "kubernetes" {
		return b, errors.New("instruction platform does not match the Kubernetes runner")
	}
	if b.SchemaVersion != 1 || !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(sourceSHA) || b.SourceSHA != sourceSHA {
		return b, errors.New("instruction bundle source revision mismatch")
	}
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(b.StaticHash) || !regexp.MustCompile(`^[a-z0-9-]{1,40}$`).MatchString(b.RunID) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(b.APIKeySHA256) || len(b.Cases) != 2 {
		return b, errors.New("invalid run identity or incomplete installation matrix")
	}
	for i, method := range []string{"operator", "helm"} {
		c := b.Cases[i]
		if c.Method != method || c.ClusterName != "onboarding-"+b.RunID+"-"+method {
			return b, errors.New("invalid installation case identity")
		}
		if _, err := commands(c.Setup); err != nil {
			return b, err
		}
		if _, err := commands(c.Install); err != nil {
			return b, err
		}
		expectedInstall := "kubectl apply -f datadog-agent.yaml"
		if method == "helm" {
			expectedInstall = "helm install datadog-agent -f datadog-values.yaml datadog/datadog --namespace datadog"
		}
		if strings.TrimSpace(c.Install) != expectedInstall || strings.Count(c.Setup, keyMarker) != 1 || strings.Contains(c.Config, keyMarker) {
			return b, errors.New("invalid install command or credentials")
		}
		if err := validateConfig(c); err != nil {
			return b, err
		}
	}
	return b, nil
}

func validateConfig(c installCase) error {
	var config map[string]any
	decoder := yaml.NewDecoder(strings.NewReader(c.Config))
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("expected one valid configuration document")
	}
	get := func(keys ...string) any {
		var value any = config
		for _, key := range keys {
			obj, ok := value.(map[string]any)
			if !ok {
				return nil
			}
			value = obj[key]
		}
		return value
	}
	if c.Method == "operator" {
		if get("kind") != "DatadogAgent" || get("apiVersion") != "datadoghq.com/v2alpha1" || get("metadata", "namespace") != "datadog" || get("spec", "global", "clusterName") != c.ClusterName || get("spec", "global", "credentials", "apiSecret", "secretName") != "datadog-secret" {
			return errors.New("unexpected DatadogAgent target or credential reference")
		}
	} else if get("datadog", "clusterName") != c.ClusterName || get("datadog", "apiKeyExistingSecret") != "datadog-secret" {
		return errors.New("unexpected Helm target or credential reference")
	}
	var site any
	if c.Method == "operator" {
		site = get("spec", "global", "site")
		if get("spec", "features", "apm", "instrumentation", "enabled") != true || get("spec", "features", "logCollection", "enabled") != true {
			return errors.New("Operator configuration lost the selected APM/Logs options")
		}
	} else {
		site = get("datadog", "site")
		if get("datadog", "apm", "instrumentation", "enabled") != true || get("datadog", "logs", "enabled") != true {
			return errors.New("Helm configuration lost the selected APM/Logs options")
		}
	}
	if site != "datadoghq.com" && site != "datad0g.com" {
		return errors.New("unapproved Datadog intake site")
	}
	if hasTLSBypass(config) {
		return errors.New("EKS installation must not disable kubelet TLS verification")
	}
	return nil
}

func hasTLSBypass(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if (key == "tlsVerify" || key == "kubelet_tls_verify") && (child == false || child == "false") {
				return true
			}
			if key == "name" && child == "DD_KUBELET_TLS_VERIFY" && (value["value"] == false || value["value"] == "false") {
				return true
			}
			if hasTLSBypass(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if hasTLSBypass(child) {
				return true
			}
		}
	}
	return false
}

func verifyKey(b bundle, apiKey string) error {
	digest := sha256.Sum256([]byte(apiKey))
	if !regexp.MustCompile(`^[a-fA-F0-9]{32}$`).MatchString(apiKey) || hex.EncodeToString(digest[:]) != b.APIKeySHA256 {
		return errors.New("test key differs from the key copied by the frontend")
	}
	return nil
}

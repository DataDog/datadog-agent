// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package clusteragentimpl implements the clusteragent metadata providers interface
package clusteragentimpl

import (
	"fmt"

	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
)

const defaultHelmDriver = "configmap"

var helmValues = ""

// retrieveHelmValues returns the Helm values for the cluster agent
// TODO:
// - Keep the settings / actionConfig in the struct to avoid re-initializing them
// - Cache the values to avoid fetching them on every request -- they can't change as long as the pod is running
func retrieveHelmValues() ([]byte, error) {
	if helmValues != "" {
		return []byte(helmValues), nil
	}

	settings := cli.New()
	settings.SetNamespace("default") // TODO: is this always where the helm release is installed?

	// Create a Helm action configuration
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), defaultHelmDriver, nil); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action configuration: %w", err)
	}

	releaseName := "datadog-agent" // TODO: Retrieve this from pod labels

	// Get values of the release
	valuesClient := action.NewGetValues(actionConfig)
	values, err := valuesClient.Run(releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Helm values for release %s: %w", releaseName, err)
	}

	// Marshal the values to a string
	valuesBytes, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Helm values: %w", err)
	}

	// Store the values in the global variable for future use
	helmValues = string(valuesBytes)

	return valuesBytes, err
}

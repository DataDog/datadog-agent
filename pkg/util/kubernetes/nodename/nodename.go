// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodename reads the name of the local Kubernetes node from the
// environment, where deployment tools put it through the downward API
// (fieldRef: spec.nodeName), so that it is known without querying the kubelet.
package nodename

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// FromEnv returns the local node's name, read from the first set environment
// variable among envVars, a comma-separated list of names, and the name of
// that variable.
//
// It returns an error when none of them is set. It also returns one when the
// value, once trimmed, isn't a valid node name, along with that value and the
// variable it was read from: node names are DNS subdomains, so anything else
// can't match a pod's spec.nodeName.
func FromEnv(envVars string) (nodeName, envVar string, err error) {
	for _, name := range strings.Split(envVars, ",") {
		name = strings.TrimSpace(name)
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			nodeName, envVar = value, name
			break
		}
	}
	if nodeName == "" {
		return "", "", fmt.Errorf("none of the environment variables in %q is set", envVars)
	}

	if problems := validation.IsDNS1123Subdomain(nodeName); len(problems) > 0 {
		return nodeName, envVar, fmt.Errorf("environment variable %q holds %q, which isn't a valid node name: %s", envVar, nodeName, strings.Join(problems, "; "))
	}

	return nodeName, envVar, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package kubernetes

import (
	"io/ioutil"

	log "github.com/cihub/seelog"
)

// Kubelet constants
const (
	AuthTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

// GetAuthToken reads the serviceaccount token
func GetAuthToken() string {
	token, err := ioutil.ReadFile(AuthTokenPath)
	if err != nil {
		log.Errorf("Could not read token from %s: %s", AuthTokenPath, err)
		return ""
	}
	return string(token)
}

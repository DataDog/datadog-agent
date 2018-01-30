// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package collectors

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// doServiceMapping TODO waiting for the DCA
func doServiceMapping() {
	apiC, err := apiserver.GetAPIClient()
	if err != nil {
		return
	}
	log.Tracef("refreshing the service mapping...")
	apiC.FetchAndStoreServiceMapping()
}

// doServiceMapping TODO waiting for the DCA
func getPodServiceNames(nodeName, podName string) []string {
	log.Tracef("getting %s/%s", nodeName, podName)
	return apiserver.GetPodServiceNames(nodeName, podName)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python,kubelet

package python

import (
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <datadog_agent_six.h>
#cgo !windows LDFLAGS: -ldatadog-agent-six -ldl
#cgo windows LDFLAGS: -ldatadog-agent-six -lstdc++ -static
*/
import "C"

var (
	kubeletCacheKey = cache.BuildAgentKey("py", "kubeutil", "connection_info")

	// for testing
	getConnectionsFunc = getConnections
)

func getConnections() map[string]string {
	kubeutil, err := kubelet.GetKubeUtil()
	if err != nil {
		// Connection to the kubelet fail, return empty dict
		log.Errorf("connection to kubelet failed: %v", err)
		return nil
	}

	// At this point, we have valid credentials to get
	return kubeutil.GetRawConnectionInfo()
}

// GetKubeletConnectionInfo returns a dict containing url and credentials to connect to the kubelet.
// The dict is empty if the kubelet was not detected. The call to kubeutil is cached for 5 minutes.
// See the documentation of kubelet.GetRawConnectionInfo for dict contents.
//export GetKubeletConnectionInfo
func GetKubeletConnectionInfo(payload **C.char) {
	var creds string
	var ok bool

	if cached, hit := cache.Cache.Get(kubeletCacheKey); hit {
		log.Debug("cache hit for kubelet connection info")
		if creds, ok = cached.(string); !ok {
			log.Error("invalid cache format, forcing a cache miss")
			creds = ""
		}
	}

	if creds == "" { // Cache miss
		log.Debug("cache miss for kubelet connection info")
		connections := getConnectionsFunc()
		if connections == nil {
			return
		}

		data, err := yaml.Marshal(connections)
		if err != nil {
			log.Errorf("could not serialized kubelet connections (%s): %s", connections, err)
			return
		}

		creds = string(data)
		cache.Cache.Set(kubeletCacheKey, creds, 5*time.Minute)
	}

	*payload = C.CString(creds)
}

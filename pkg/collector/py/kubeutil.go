// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython,kubelet

package py

import (
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// #cgo pkg-config: python-2.7
// #cgo linux CFLAGS: -std=gnu99
// #include "api.h"
// #include "kubeutil.h"
import "C"

var kubeletCacheKey = cache.BuildAgentKey("py", "kubeutil", "connection_info")

// GetKubeletConnectionInfo returns a dict containing url and credentials to connect to the kubelet.
// The dict is empty if the kubelet was not detected. The call to kubeutil is cached for 5 minutes.
// See the documentation of kubelet.GetRawConnectionInfo for dict contents.
//export GetKubeletConnectionInfo
func GetKubeletConnectionInfo() *C.PyObject {
	var creds map[string]string
	var ok bool
	dict := C.PyDict_New()

	if cached, hit := cache.Cache.Get(kubeletCacheKey); hit {
		log.Debug("cache hit for kubelet connection info")
		if creds, ok = cached.(map[string]string); !ok {
			log.Error("invalid cache format, forcing a cache miss")
			creds = nil
		}
	}

	if creds == nil { // Cache miss
		log.Debug("cache miss for kubelet connection info")
		kubeutil, err := kubelet.GetKubeUtil()
		if err != nil {
			// Connection to the kubelet fail, return empty dict
			log.Errorf("connection to kubelet failed: %v", err)
			return dict
		}
		// At this point, we have valid credentials to get
		creds = kubeutil.GetRawConnectionInfo()
		cache.Cache.Set(kubeletCacheKey, creds, 5*time.Minute)
	}

	for k, v := range creds {
		cKey := C.CString(k)
		pyKey := C.PyString_FromString(cKey)
		defer C.Py_DecRef(pyKey)
		C.free(unsafe.Pointer(cKey))

		cVal := C.CString(v)
		pyVal := C.PyString_FromString(cVal)
		defer C.Py_DecRef(pyVal)
		C.free(unsafe.Pointer(cVal))

		C.PyDict_SetItem(dict, pyKey, pyVal)
	}
	return dict
}

func initKubeutil() {
	C.initkubeutil()
}

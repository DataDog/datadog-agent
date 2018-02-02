// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !kubeapiserver

package collectors

import (
	log "github.com/cihub/seelog"
)

// addToCacheServiceMapping TODO waiting for the DCA
func doServiceMapping(interfaces ...interface{}) {
	log.Error("nocompile package")
}

// addToCacheServiceMapping TODO waiting for the DCA
func getPodServiceNames(interfaces ...interface{}) []string {
	log.Error("nocompile package")
	return nil
}

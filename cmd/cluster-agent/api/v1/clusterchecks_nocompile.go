// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !clusterchecks

package v1

import (
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
)

func installClusterCheckEndpoints(r *mux.Router) {
}

// SetClusterCheckHandler not implemented
func SetClusterCheckHandler(h *clusterchecks.Handler) {
}

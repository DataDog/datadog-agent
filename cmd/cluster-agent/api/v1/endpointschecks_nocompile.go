// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !clusterchecks

package v1

import (
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
)

// installEndpointsCheckEndpoints not implemented
func installEndpointsCheckEndpoints(_ *mux.Router, _ clusteragent.ServerContext) {}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build !clusterchecks && !kubeapiserver

package v1

import (
	"github.com/gorilla/mux"
)

func installCloudFoundryMetadataEndpoints(r *mux.Router) {}

func installKubernetesMetadataEndpoints(r *mux.Router) {}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build !clusterchecks && !kubeapiserver

package v1

import (
	"net/http"

	clusteridresolver "github.com/DataDog/datadog-agent/comp/core/clusteridresolver/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func installCloudFoundryMetadataEndpoints(_ *http.ServeMux) {}

func installKubernetesMetadataEndpoints(_ *http.ServeMux, _ workloadmeta.Component, _ clusteridresolver.Component) {
}

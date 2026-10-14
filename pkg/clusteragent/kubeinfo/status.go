// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeinfo

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider adds the Kubernetes information to the status header.
type Provider struct{}

// Index returns the index
func (Provider) Index() int {
	return 2
}

// Name returns the name
func (Provider) Name() string {
	return "Kubernetes Information"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	stats["kubernetesInfo"] = getStatusInfo()
	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "kubeinfo.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func getStatusInfo() map[string]interface{} {
	i := getInfo()

	versionInfo := map[string]interface{}{
		"detected":     i.serverVersion,
		"minSupported": common.MinSupportedKubeVersion,
	}
	if i.unsupported {
		versionInfo["warning"] = warningMessage(i.serverVersion)
	}

	return map[string]interface{}{
		"clusterName":  i.clusterName,
		"clusterID":    i.clusterID,
		"distribution": i.distribution,
		"version":      versionInfo,
	}
}

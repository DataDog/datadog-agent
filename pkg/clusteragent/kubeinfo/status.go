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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider adds the Kubernetes information to the status header.
type Provider struct {
	wmeta workloadmeta.Component
}

// NewProvider returns a Provider
func NewProvider(wmeta workloadmeta.Component) Provider {
	return Provider{wmeta: wmeta}
}

// Index returns the index
func (Provider) Index() int {
	return 2
}

// Name returns the name
func (Provider) Name() string {
	return "Kubernetes Information"
}

// JSON populates the status map
func (p Provider) JSON(_ bool, stats map[string]interface{}) error {
	stats["kubernetesInfo"] = p.getStatusInfo()
	return nil
}

// Text renders the text output
func (p Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "kubeinfo.tmpl", buffer, p.getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func (p Provider) getStatusInfo() map[string]interface{} {
	versionInfo := map[string]interface{}{
		"detected":     "",
		"minSupported": common.MinSupportedKubeVersion,
	}
	// The version is collected by workloadmeta: it is unknown until collected.
	if kubeCapabilities, err := p.wmeta.GetKubeCapabilities(); err == nil {
		detected, unsupported := versionStatus(kubeCapabilities.Version)
		versionInfo["detected"] = detected
		if unsupported {
			versionInfo["warning"] = warningMessage(detected)
		}
	}

	i := getClusterInfo()
	return map[string]interface{}{
		"clusterName":  i.clusterName,
		"clusterID":    i.clusterID,
		"distribution": i.distribution,
		"version":      versionInfo,
	}
}

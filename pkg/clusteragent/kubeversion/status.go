// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeversion

import (
	"embed"
	"io"

	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

//go:embed status_templates
var templatesFS embed.FS

// Provider adds the unsupported Kubernetes version warning to the status header.
// It is hidden when the version is supported or unknown.
type Provider struct{}

// Index returns the index
func (Provider) Index() int {
	return 2
}

// Name returns the name. An empty name hides the section.
func (Provider) Name() string {
	if getUnsupportedVersion() == "" {
		return ""
	}
	return "Kubernetes Version"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	if info := getStatusInfo(); info != nil {
		stats["kubernetesVersion"] = info
	}
	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	info := getStatusInfo()
	if info == nil {
		return nil
	}
	return status.RenderText(templatesFS, "kubeversion.tmpl", buffer, info)
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func getStatusInfo() map[string]interface{} {
	detected := getUnsupportedVersion()
	if detected == "" {
		return nil
	}
	return map[string]interface{}{
		"detectedVersion":     detected,
		"minSupportedVersion": common.MinSupportedKubeVersion,
		"supported":           false,
		"warning":             warningMessage(detected),
	}
}

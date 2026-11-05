// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeinfo

import (
	"context"
	"embed"
	"io"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

//go:embed status_templates
var templatesFS embed.FS

// statusTimeout bounds the lookups done when rendering the status.
const statusTimeout = 2 * time.Second

// Provider adds the Kubernetes information to the status header.
type Provider struct {
	wmeta    workloadmeta.Component
	hostname hostnameinterface.Component

	getClusterName  func(ctx context.Context, hostname string) string
	getClusterID    func() (string, error)
	getDistribution func(ctx context.Context) string
}

// NewProvider returns a Provider
func NewProvider(wmeta workloadmeta.Component, hostname hostnameinterface.Component) Provider {
	return Provider{
		wmeta:           wmeta,
		hostname:        hostname,
		getClusterName:  clustername.GetRFC1123CompliantClusterName,
		getClusterID:    clustername.GetClusterID,
		getDistribution: cloudprovider.DCAGetName,
	}
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

// getStatusInfo gets the information on demand: the cluster agent computes and
// caches it at startup, a failed cluster ID or distribution lookup is retried.
func (p Provider) getStatusInfo() map[string]interface{} {
	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()

	versionInfo := map[string]interface{}{
		"detected":     "",
		"minSupported": minSupportedKubeVersion,
	}
	// The version is collected by workloadmeta: it is unknown until collected.
	if kubeCapabilities, err := p.wmeta.GetKubeCapabilities(); err == nil {
		detected, unsupported := versionStatus(kubeCapabilities.Version)
		versionInfo["detected"] = detected
		if unsupported {
			versionInfo["warning"] = warningMessage(detected)
		}
	}

	hostname, _ := p.hostname.Get(ctx)
	clusterID, _ := p.getClusterID()
	return map[string]interface{}{
		"clusterName":  p.getClusterName(ctx, hostname),
		"clusterID":    clusterID,
		"distribution": p.getDistribution(ctx),
		"version":      versionInfo,
	}
}

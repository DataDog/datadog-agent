// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/wlm"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_linux_mock.go

func init() {
	newOSImpl = newLinuxImpl
}

type linuxImpl struct {
	getDiscoveryServices func(client *sysprobeclient.CheckClient) (*model.ServicesResponse, error)
	sysProbeClient       *sysprobeclient.CheckClient
}

func newLinuxImpl(store workloadmeta.Component, tagger tagger.Component) (osImpl, error) {
	useWorkloadmeta := pkgconfigsetup.SystemProbe().GetBool("discovery.use_workloadmeta")
	log.Info("useWorkloadmeta", "useWorkloadmeta", useWorkloadmeta)
	if useWorkloadmeta {
		discoveryWLM, err := wlm.NewDiscoveryWLM(store, tagger)
		if err != nil {
			return nil, err
		}
		return &linuxImpl{
			getDiscoveryServices: func(_ *sysprobeclient.CheckClient) (*model.ServicesResponse, error) {
				return discoveryWLM.DiscoverServices()
			},
		}, nil
	}

	return &linuxImpl{
		getDiscoveryServices: getDiscoveryServices,
		sysProbeClient:       sysprobeclient.GetCheckClient(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	}, nil
}

func getDiscoveryServices(client *sysprobeclient.CheckClient) (*model.ServicesResponse, error) {
	resp, err := sysprobeclient.GetCheck[model.ServicesResponse](client, sysconfig.DiscoveryModule)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (li *linuxImpl) DiscoverServices() (*model.ServicesResponse, error) {
	return li.getDiscoveryServices(li.sysProbeClient)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_linux_mock.go

func init() {
	newOSImpl = newLinuxImpl
}

type linuxImpl struct {
	getDiscoveryServices func(client *sysprobeclient.CheckClient) (*model.ServicesResponse, error)
	sysProbeClient       *sysprobeclient.CheckClient
}

func newLinuxImpl() (osImpl, error) {
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

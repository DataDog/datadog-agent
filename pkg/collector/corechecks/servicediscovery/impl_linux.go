// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"encoding/json"
	"fmt"
	"net/http"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=impl_linux_mock.go

func init() {
	newOSImpl = newLinuxImpl
}

type linuxImpl struct {
	getDiscoveryServices func(client *http.Client) (*model.ServicesResponse, error)
	time                 timer

	sysProbeClient *http.Client
}

func newLinuxImpl() (osImpl, error) {
	return &linuxImpl{
		getDiscoveryServices: getDiscoveryServices,
		time:                 realTime{},
		sysProbeClient:       sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	}, nil
}

func getDiscoveryServices(client *http.Client) (*model.ServicesResponse, error) {
	url := sysprobeclient.ModuleURL(sysconfig.DiscoveryModule, "/services")
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got non-success status code: url: %s, status_code: %d", req.URL, resp.StatusCode)
	}

	res := &model.ServicesResponse{}
	if err := json.NewDecoder(resp.Body).Decode(res); err != nil {
		return nil, err
	}
	return res, nil
}

func (li *linuxImpl) DiscoverServices() (*model.ServicesResponse, error) {
	response, err := li.getDiscoveryServices(li.sysProbeClient)
	if err != nil {
		return nil, err
	}

	return response, nil
}

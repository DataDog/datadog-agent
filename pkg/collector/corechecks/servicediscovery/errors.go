// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
)

type errCode string

const (
	errorCodePortPoller          errCode = "port_poller"
	errorCodeRepeatedServiceName errCode = "repeated_service_name"
	errorCodeSystemProbeConn     errCode = "system_probe_conn"
	errorCodeSystemProbeServices errCode = "system_probe_services"
)

type errWithCode struct {
	err  error
	code errCode
	svc  *model.Service
}

func (e errWithCode) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.err.Error())
}

func (e errWithCode) Code() errCode {
	return e.code
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"fmt"
)

type errCode string

const (
	errorCodeProcfs              errCode = "procfs"
	errorCodeSystemProbeConn     errCode = "system_probe"
	errorCodeSystemProbePorts    errCode = "system_probe_ports"
	errorCodeSystemProbeGetProc  errCode = "system_probe_get_proc"
	errorCodeEmptyServiceName    errCode = "empty_service_name"
	errorCodeRepeatedServiceName errCode = "repeated_service_name"
)

type errWithCode struct {
	err  error
	code errCode
	svc  *serviceMetadata
}

func (e errWithCode) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.err.Error())
}

func (e errWithCode) Code() errCode {
	return e.code
}

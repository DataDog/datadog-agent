// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package connectivity

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
)

// EventPlatformDiagnose enumerates known epforwarder pipelines and endpoints to test each of them connectivity
func EventPlatformDiagnose() []diagnosis.Diagnosis {
	var diagnoses []diagnosis.Diagnosis

	for _, desc := range pipeline.PassthroughPipelineDescs {
		configKeys := config.NewLogsConfigKeys(desc.EndpointsConfigPrefix, pkgconfigsetup.Datadog())
		endpoints, err := config.BuildHTTPEndpointsWithConfig(pkgconfigsetup.Datadog(), configKeys, desc.HostnameEndpointPrefix, desc.IntakeTrackType, config.DefaultIntakeProtocol, config.DefaultIntakeOrigin)
		if err != nil {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Result:      diagnosis.DiagnosisFail,
				Name:        "Endpoints configuration",
				Diagnosis:   "Misconfiguration of agent endpoints",
				Remediation: "Please validate Agent configuration",
				RawError:    err.Error(),
			})
			continue
		}

		url, err := logshttp.CheckConnectivityDiagnose(endpoints.Main, pkgconfigsetup.Datadog())
		name := fmt.Sprintf("Connectivity to %s", url)
		if err == nil {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Result:    diagnosis.DiagnosisSuccess,
				Category:  desc.Category,
				Name:      name,
				Diagnosis: fmt.Sprintf("Connectivity to `%s` is Ok", url),
			})
		} else {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Result:      diagnosis.DiagnosisFail,
				Category:    desc.Category,
				Name:        name,
				Diagnosis:   fmt.Sprintf("Connection to `%s` failed", url),
				Remediation: "Please validate Agent configuration and firewall to access " + url,
				RawError:    err.Error(),
			})
		}
	}

	return diagnoses
}

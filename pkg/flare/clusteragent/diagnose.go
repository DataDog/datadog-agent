// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"bufio"
	"bytes"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/diagnose/format"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
)

// GetClusterAgentDiagnose dumps the diagnose checks to the writer
func GetClusterAgentDiagnose(diagnoseComp diagnose.Component) ([]byte, error) {
	return diagnoseComp.RunSuites("text", true)
}

// GetLocalClusterAgentDiagnose dumps the connectivity checks diagnose to the writer
func GetLocalClusterAgentDiagnose(diagnoseComp diagnose.Component) ([]byte, error) {
	suite := diagnose.Suites{
		diagnose.AutodiscoveryConnectivity: func(_ diagnose.Config) []diagnose.Diagnosis {
			return connectivity.DiagnoseMetadataAutodiscoveryConnectivity()
		},
	}

	config := diagnose.Config{Verbose: true}

	result, err := diagnoseComp.RunLocalSuite(suite, config)

	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	writer := bufio.NewWriter(&b)

	err = format.Text(writer, config, result)

	if err != nil {
		return nil, err
	}
	writer.Flush()
	return b.Bytes(), nil
}

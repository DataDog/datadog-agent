// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package flare

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

var diagnoseNames = []string{
	apiserver.DiagnoseName,
}

// GetClusterAgentDiagnose dumps the connectivity checks diagnose to the writer
func GetClusterAgentDiagnose(w io.Writer) error {
	err := diagnose.Run(w, diagnoseNames)
	return err
}

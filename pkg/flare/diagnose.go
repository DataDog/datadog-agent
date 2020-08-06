// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"io"

	"github.com/StackVista/stackstate-agent/pkg/diagnose"
)

// GetClusterAgentDiagnose dumps the connectivity checks diagnose to the writer
func GetClusterAgentDiagnose(w io.Writer) error {
	return diagnose.RunAll(w)
}

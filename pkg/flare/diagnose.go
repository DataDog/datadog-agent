// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

// GetClusterAgentDiagnose dumps the connectivity checks diagnose to the writer
func GetClusterAgentDiagnose(w io.Writer, senderManager sender.SenderManager) error {
	// Verbose:  true - to show details like if was done a while ago
	// RunLocal: true - do not attept to run in actual running agent but
	//                  may need to implement it in future
	// Include: connectivity-datadog-autodiscovery - limit to a single
	//                  diagnose suite as it was done in this agent for
	//                  a while. Most likely need to relax or add more
	//                  diagnose suites in the future
	diagCfg := diagnosis.Config{
		Verbose:  true,
		RunLocal: true,
		Include:  []string{"connectivity-datadog-autodiscovery"},
	}
	return diagnose.RunStdOut(w, diagCfg, senderManager)
}

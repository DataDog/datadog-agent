// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
)

// clearHealth serves POST /logs/clear-health for `agent logs clear-health`. It only
// forgets the loss incidents: the health checks resolve the issues on their next
// run, and an issue whose cause is not fixed comes back instead of flapping.
func clearHealth(w http.ResponseWriter, _ *http.Request) {
	metrics.ClearLogsHealth()
	w.WriteHeader(http.StatusNoContent)
}

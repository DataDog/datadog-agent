// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func getVerboseWorkloadList(w http.ResponseWriter, r *http.Request) {
	workloadList(w, true)
}

func getShortWorkloadList(w http.ResponseWriter, r *http.Request) {
	workloadList(w, false)
}

func workloadList(w http.ResponseWriter, verbose bool) {
	response := workloadmeta.GetGlobalStore().Dump(verbose)
	jsonDump, err := json.Marshal(response)
	if err != nil {
		setJSONError(w, log.Errorf("Unable to marshal workload list response: %v", err), 500)
		return
	}

	w.Write(jsonDump)
}

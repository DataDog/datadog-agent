// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm)

package modules

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func writeDisabledProtocolMessage(protocolName string, w http.ResponseWriter) {
	log.Warnf("%s monitoring is disabled", protocolName)
	w.WriteHeader(404)
	// Writing JSON to ensure compatibility when using the jq bash utility for output
	outputString := map[string]string{"error": protocolName + " monitoring is disabled"}
	// We are marshaling a static string, so we can ignore the error
	buf, _ := json.Marshal(outputString)
	w.Write(buf)
}

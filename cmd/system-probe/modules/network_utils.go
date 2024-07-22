// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || darwin

package modules

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/network"
)

func getClientID(req *http.Request) string {
	var clientID = network.DEBUGCLIENT
	if rawCID := req.URL.Query().Get("client_id"); rawCID != "" {
		clientID = rawCID
	}
	return clientID
}

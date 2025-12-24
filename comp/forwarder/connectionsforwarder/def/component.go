// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package connectionsforwarder defines a component to send connections data to the backend
package connectionsforwarder

import (
	"net/http"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// team: container-experiences

// Component describes the interface implemented by connections forwarder implementations
type Component interface {
	SubmitConnectionChecks(payload transaction.BytesPayloads, extra http.Header) (chan defaultforwarder.Response, error)
}

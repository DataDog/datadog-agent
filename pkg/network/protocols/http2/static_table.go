// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	nethttp "net/http"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/util/intern"
)

var (
	// methodStaticTable maps the methods in the static table, to a string representation.
	methodStaticTable = map[uint8]http.Method{
		GetValue:  http.MethodGet,
		PostValue: http.MethodPost,
	}

	// statusStaticTable maps the kernel enum value to a HTTP status code.
	statusStaticTable = map[uint8]uint16{
		K200Value: nethttp.StatusOK,
		K204Value: nethttp.StatusNoContent,
		K206Value: nethttp.StatusPartialContent,
		K304Value: nethttp.StatusNotModified,
		K400Value: nethttp.StatusBadRequest,
		K404Value: nethttp.StatusNotFound,
		K500Value: nethttp.StatusInternalServerError,
	}

	// pathStaticTable maps the kernel enum values to the string representation in the static table.
	pathStaticTable = map[uint8]*intern.StringValue{
		EmptyPathValue: interner.GetString("/"),
		IndexPathValue: interner.GetString("/index.html"),
	}
)

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package network

import "github.com/DataDog/datadog-agent/pkg/network/protocols/http"

// HTTPKeyTuplesFromConn build the key for the http map based on whether the local or remote side is http.
func HTTPKeyTuplesFromConn(c ConnectionStats) []http.KeyTuple {

	laddr := c.Source
	lport := c.SPort
	raddr := c.Dest
	rport := c.DPort

	return []http.KeyTuple{
		http.NewKeyTuple(laddr, raddr, lport, rport),
	}

}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"github.com/google/gopacket"
)

type Packet interface {
	GetCaptureInfo() *gopacket.CaptureInfo
	GetData() []byte
}

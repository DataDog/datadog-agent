// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// RawEncoder is a shared raw encoder.
var RawEncoder Encoder = &rawEncoder{}

type rawEncoder struct{}

func (r *rawEncoder) Encode(msg *message.Message) error {
	panic("not called")
}

var rfc5424Pattern, _ = regexp.Compile("<[0-9]{1,3}>[0-9] ")

func isRFC5424Formatted(content []byte) bool {
	panic("not called")
}

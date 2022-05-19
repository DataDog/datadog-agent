// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"time"

	"github.com/mailru/easyjson/jwriter"
)

type EasyjsonTime time.Time

func (t EasyjsonTime) MarshalEasyJSON(w *jwriter.Writer) {
	tt := time.Time(t)
	if y := tt.Year(); y < 0 || y >= 10000 {
		if w.Error == nil {
			w.Error = errors.New("Time.MarshalJSON: year outside of range [0,9999]")
		}
		return
	}

	w.Buffer.EnsureSpace(len(time.RFC3339Nano) + 2)
	w.Buffer.AppendByte('"')
	w.Buffer.Buf = tt.AppendFormat(w.Buffer.Buf, time.RFC3339Nano)
	w.Buffer.AppendByte('"')
}

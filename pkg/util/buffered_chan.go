// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package util

import (
	"github.com/DataDog/datadog-agent/pkg/util/buf"
)

// BufferedChan behaves like a `chan []interface{}` (See thread safety for restrictions), but is
// most efficient as it uses internally a channel of []interface{}. This reduces the number of reads
// and writes to the channel. Instead of having one write and one read for each value for a regular channel,
// there are one write and one read for each `bufferSize` value.
// Thread safety:
//   - `BufferedChan.Put` cannot be called concurrently.
//   - `BufferedChan.Get` cannot be called concurrently.
//   - `BufferedChan.Put` can be called while another goroutine calls `BufferedChan.Get`.
type BufferedChan = buf.BufferedChan

// NewBufferedChan creates a new instance of `BufferedChan`.
// `ctx` can be used to cancel all Put and Get operations.
var NewBufferedChan = buf.NewBufferedChan

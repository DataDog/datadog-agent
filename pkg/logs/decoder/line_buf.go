// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package decoder

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
)

// lineBuffer defines the fields keep tracking the status of each transition in decoding
// process.
type lineBuffer struct {
	// lastLeading is the needLeading status from the previous line.
	lastLeading bool
	// lastTailing is the needTailing status from the previous line.
	lastTailing bool
	// contentBuf keeps the contents which can not be sent to the next pipe.
	contentBuf *bytes.Buffer
	// lastPrefix is the Prefix of previous line.
	lastPrefix parser.Prefix
}

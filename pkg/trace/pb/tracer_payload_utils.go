// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pb

import (
	"github.com/DataDog/datadog-agent/pkg/proto/utils"
)

var copyTraceChunk = utils.ProtoCopier((*TraceChunk)(nil))

func (t *TraceChunk) ShallowCopy() *TraceChunk { return copyTraceChunk(t).(*TraceChunk) }

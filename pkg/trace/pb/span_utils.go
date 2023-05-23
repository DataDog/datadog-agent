// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pb

import (
	"github.com/DataDog/datadog-agent/pkg/proto/utils"
)

var copySpan = utils.ProtoCopier((*Span)(nil))

func (s *Span) ShallowCopy() *Span { return copySpan(s).(*Span) }

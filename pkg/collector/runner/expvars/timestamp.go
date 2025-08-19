// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvars

import (
	"fmt"
	"time"
)

type timestamp time.Time

func (t timestamp) String() string {
	return fmt.Sprintf("\"%s\"", time.Time(t).Format(time.RFC3339))
}

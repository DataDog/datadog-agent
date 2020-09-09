// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package remote

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func LogEntry(logType, flareId, tracerId string, data io.ReadCloser) error {
}

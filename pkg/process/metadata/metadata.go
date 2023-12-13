// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import "github.com/DataDog/datadog-agent/pkg/process/procutil"

// Extractor is common interface for extracting metadata from processes
type Extractor interface {
	Extract(procs map[int32]*procutil.Process)
}

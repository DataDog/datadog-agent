// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

import "errors"

// ErrAttachBPFNotSupported indicates that attaching classic BPF filters is not supported
var ErrAttachBPFNotSupported = errors.New("attaching BPF filters is not supported on this platform")

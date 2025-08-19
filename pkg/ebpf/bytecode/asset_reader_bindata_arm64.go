// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpf_bindata && arm64

package bytecode

import (
	"embed"
)

//go:embed build/arm64/runtime-security.o
//go:embed build/arm64/runtime-security-syscall-wrapper.o
//go:embed build/arm64/runtime-security-fentry.o
//go:embed build/arm64/runtime-security-offset-guesser.o
var bindata embed.FS

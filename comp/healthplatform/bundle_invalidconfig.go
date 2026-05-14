// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package healthplatform

// Side-effect import lives in its own file so the !jetson tag can keep the
// schema validator out of IoT Agent builds.
import _ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig" // registers the invalidconfig issue module via init()

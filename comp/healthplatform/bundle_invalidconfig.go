// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package healthplatform

// Side-effect import: registers the invalidconfig issue module via its init().
// Lives in its own file so the !jetson tag can keep the schema validator
// out of the IoT Agent binary.
import _ "github.com/DataDog/datadog-agent/comp/healthplatform/issues/invalidconfig"

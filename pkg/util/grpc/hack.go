// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build hack_no_include

package grpc

// In order to fix the ambuiguity between old versions of cloud.google.com/go
// and recent versions of cloud.google.com/go/compute/metadate providing the same
// package, we force a dependency on a newer version of cloud.google.com/go here.
// Thanks to the build tag, it will not be included in the final binary.

import _ "cloud.google.com/go/compute/apiv1"

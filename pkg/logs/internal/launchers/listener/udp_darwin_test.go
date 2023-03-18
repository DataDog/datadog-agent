// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package listener

// TODO: Get it from `sysctl sys.inet.udp.maxdgram`
const maxUDPFrameLen = 9216

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import "flag"

var (
	osDescriptors             = flag.String("osdescriptors", "", "platform/arch/os version (debian/x86_64/11)")
	cwsSupportedOsDescriptors = flag.String("cws-supported-osdescriptors", "", "list of os descriptors where CWS is supported")
	flavorName                = flag.String("flavor", "datadog-agent", "package flavor to install")
	majorVersion              = flag.String("major-version", "7", "major version to test (6, 7)")
	srcAgentVersion           = flag.String("src-agent-version", "7", "start agent version")
	destAgentVersion          = flag.String("dest-agent-version", "7", "destination agent version to upgrade to")
)

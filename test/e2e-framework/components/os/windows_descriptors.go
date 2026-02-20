// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

// Implements commonly used descriptors for easier usage
// See platforms.go for the AMIs used for each OS
var (
	WindowsServerDefault = WindowsServer2025
	WindowsServer2025    = NewDescriptor(WindowsServer, "2025")
	WindowsServer2022    = NewDescriptor(WindowsServer, "2022")
	WindowsServer2019    = NewDescriptor(WindowsServer, "2019")
	WindowsServer2016    = NewDescriptor(WindowsServer, "2016")

	WindowsClientDefault = WindowsClient1124H2
	WindowsClient11      = WindowsClient1124H2
	WindowsClient1124H2  = NewDescriptor(WindowsClient, "windows-11:win11-24h2-pro")
	WindowsClient1122H2  = NewDescriptor(WindowsClient, "windows-11:win11-22h2-pro")
	WindowsClient10      = WindowsClient1022H2
	WindowsClient1022H2  = NewDescriptor(WindowsClient, "windows-10:win10-22h2-pro")
	WindowsClient1021H2  = NewDescriptor(WindowsClient, "windows-10:win10-21h2-pro")
	WindowsClient1019H1  = NewDescriptor(WindowsClient, "Windows-10:19h1-pro-gensecond")
)

var WindowsDescriptorsDefault = map[Flavor]Descriptor{
	WindowsServer: WindowsServerDefault,
	WindowsClient: WindowsClientDefault,
}

// WindowsServerVersionsForE2E is the set of Windows Server versions used for e2e random selection (CI coverage across 2016–2025).
var WindowsServerVersionsForE2E = []Descriptor{
	WindowsServer2016,
	WindowsServer2019,
	WindowsServer2022,
	WindowsServer2025,
}

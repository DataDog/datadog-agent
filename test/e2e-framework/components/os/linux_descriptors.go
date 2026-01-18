// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

// Implements commonly used descriptors for easier usage
// See platforms.go for the AMIs used for each OS
var (
	UbuntuDefault = Ubuntu2204
	Ubuntu2404    = NewDescriptor(Ubuntu, "24-04")
	Ubuntu2204    = NewDescriptor(Ubuntu, "22-04")
	Ubuntu2004    = NewDescriptor(Ubuntu, "20-04")

	DebianDefault = Debian12
	Debian12      = NewDescriptor(Debian, "12")

	AmazonLinuxDefault = AmazonLinux2023
	AmazonLinux2023    = NewDescriptor(AmazonLinux, "2023")
	AmazonLinux2       = NewDescriptor(AmazonLinux, "2")
	AmazonLinux2018    = NewDescriptor(AmazonLinux, "2018")

	AmazonLinuxECSDefault = AmazonLinuxECS2
	AmazonLinuxECS2023    = NewDescriptor(AmazonLinuxECS, "2023")
	AmazonLinuxECS2       = NewDescriptor(AmazonLinuxECS, "2")

	RedHatDefault = RedHat9
	RedHat9       = NewDescriptor(RedHat, "9")

	SuseDefault = Suse15
	Suse15      = NewDescriptor(Suse, "15-4")

	FedoraDefault = Fedora40
	Fedora40      = NewDescriptor(Fedora, "40")

	CentOSDefault = CentOS7
	CentOS7       = NewDescriptor(CentOS, "79")
)

var LinuxDescriptorsDefault = map[Flavor]Descriptor{
	Ubuntu:         UbuntuDefault,
	AmazonLinux:    AmazonLinuxDefault,
	AmazonLinuxECS: AmazonLinuxECSDefault,
	Debian:         DebianDefault,
	RedHat:         RedHatDefault,
	Suse:           SuseDefault,
	Fedora:         FedoraDefault,
	CentOS:         CentOSDefault,
}

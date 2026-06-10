// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package os

// Implements commonly used descriptors for easier usage
// See platforms.go for the AMIs used for each OS
var (
	UbuntuDefault    = Ubuntu2204E2E
	Ubuntu2404       = NewDescriptor(Ubuntu, "24-04")
	Ubuntu2204       = NewDescriptor(Ubuntu, "22-04")
	Ubuntu2004       = NewDescriptor(Ubuntu, "20-04")
	Ubuntu2204E2E    = NewDescriptor(Ubuntu, "22-04-e2e")
	Ubuntu2204E2EARM = NewDescriptorWithArch(Ubuntu, "22-04-e2e", ARM64Arch)
	Ubuntu2404E2E    = NewDescriptor(Ubuntu, "24-04-e2e")
	Ubuntu2404E2EARM = NewDescriptorWithArch(Ubuntu, "24-04-e2e", ARM64Arch)

	DebianDefault = Debian12
	Debian12      = NewDescriptor(Debian, "12")
	Debian12E2E   = NewDescriptor(Debian, "12-e2e")

	AmazonLinuxDefault = AmazonLinux2023
	AmazonLinux2023    = NewDescriptor(AmazonLinux, "2023")
	AmazonLinux2       = NewDescriptor(AmazonLinux, "2")
	AmazonLinux2018    = NewDescriptor(AmazonLinux, "2018")
	AmazonLinux2E2E    = NewDescriptor(AmazonLinux, "2-e2e")
	AmazonLinux2E2EARM = NewDescriptorWithArch(AmazonLinux, "2-e2e", ARM64Arch)

	AmazonLinuxECSDefault = AmazonLinuxECS2
	AmazonLinuxECS2023    = NewDescriptor(AmazonLinuxECS, "2023")
	AmazonLinuxECS2       = NewDescriptor(AmazonLinuxECS, "2")

	RedHatDefault = RedHat9
	RedHat9       = NewDescriptor(RedHat, "9")
	RedHat9E2E    = NewDescriptor(RedHat, "9-e2e")
	RedHat9E2EARM = NewDescriptorWithArch(RedHat, "9-e2e", ARM64Arch)

	SuseDefault   = Suse15
	Suse15        = NewDescriptor(Suse, "15-4")
	Suse154E2E    = NewDescriptor(Suse, "15-4-e2e")
	Suse154E2EARM = NewDescriptorWithArch(Suse, "15-4-e2e", ARM64Arch)

	FedoraDefault = Fedora40
	Fedora40      = NewDescriptor(Fedora, "40")

	CentOSDefault = CentOS7
	CentOS7       = NewDescriptor(CentOS, "79")
	CentOS7E2E    = NewDescriptor(CentOS, "7-e2e")
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

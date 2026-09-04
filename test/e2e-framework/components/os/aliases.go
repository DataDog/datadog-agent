// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package os

// The Pulumi-free OS descriptors now live in the types subpackage; the aliases
// below keep every existing import of this package working. The Pulumi-backed
// package-manager machinery and OS constructors stay here.

import "github.com/DataDog/datadog-agent/test/e2e-framework/components/os/types"

var AMD64Arch = types.AMD64Arch
var APTDisableUnattendedUpgradesScriptContent = types.APTDisableUnattendedUpgradesScriptContent
var ARM64Arch = types.ARM64Arch
var AlmaLinux = types.AlmaLinux
var AlmaLinux9 = types.AlmaLinux9
var AmazonLinux = types.AmazonLinux
var AmazonLinux2 = types.AmazonLinux2
var AmazonLinux2018 = types.AmazonLinux2018
var AmazonLinux2023 = types.AmazonLinux2023
var AmazonLinuxDefault = types.AmazonLinuxDefault
var AmazonLinuxECS = types.AmazonLinuxECS
var AmazonLinuxECS2 = types.AmazonLinuxECS2
var AmazonLinuxECS2023 = types.AmazonLinuxECS2023
var AmazonLinuxECSDefault = types.AmazonLinuxECSDefault
type Architecture = types.Architecture
var ArchitectureFromString = types.ArchitectureFromString
var CentOS = types.CentOS
var CentOS7 = types.CentOS7
var CentOSDefault = types.CentOSDefault
var Debian = types.Debian
var Debian12 = types.Debian12
var DebianDefault = types.DebianDefault
type Descriptor = types.Descriptor
var DescriptorFromString = types.DescriptorFromString
type Family = types.Family
var Fedora = types.Fedora
var Fedora40 = types.Fedora40
var FedoraDefault = types.FedoraDefault
type Flavor = types.Flavor
var FlavorFromString = types.FlavorFromString
var LinuxDescriptorsDefault = types.LinuxDescriptorsDefault
var LinuxFamily = types.LinuxFamily
var MacOSDefault = types.MacOSDefault
var MacOSDescriptorsDefault = types.MacOSDescriptorsDefault
var MacOSFamily = types.MacOSFamily
var MacOSSonoma = types.MacOSSonoma
var MacosOS = types.MacosOS
var NewDescriptor = types.NewDescriptor
var NewDescriptorWithArch = types.NewDescriptorWithArch
var RedHat = types.RedHat
var RedHat10 = types.RedHat10
var RedHat8 = types.RedHat8
var RedHat9 = types.RedHat9
var RedHatDefault = types.RedHatDefault
var RockyLinux = types.RockyLinux
var SSHAllowSFTPRootScriptContent = types.SSHAllowSFTPRootScriptContent
var Suse = types.Suse
var Suse15 = types.Suse15
var SuseDefault = types.SuseDefault
var Ubuntu = types.Ubuntu
var Ubuntu2004 = types.Ubuntu2004
var Ubuntu2204 = types.Ubuntu2204
var Ubuntu2204E2E = types.Ubuntu2204E2E
var Ubuntu2204E2EARM = types.Ubuntu2204E2EARM
var Ubuntu2404 = types.Ubuntu2404
var Ubuntu2404E2E = types.Ubuntu2404E2E
var UbuntuDefault = types.UbuntuDefault
var Unknown = types.Unknown
var UnknownFamily = types.UnknownFamily
var WindowsClient = types.WindowsClient
var WindowsClient10 = types.WindowsClient10
var WindowsClient1019H1 = types.WindowsClient1019H1
var WindowsClient1021H2 = types.WindowsClient1021H2
var WindowsClient1022H2 = types.WindowsClient1022H2
var WindowsClient11 = types.WindowsClient11
var WindowsClient1122H2 = types.WindowsClient1122H2
var WindowsClient1124H2 = types.WindowsClient1124H2
var WindowsClientDefault = types.WindowsClientDefault
var WindowsDescriptorsDefault = types.WindowsDescriptorsDefault
var WindowsFamily = types.WindowsFamily
var WindowsServer = types.WindowsServer
var WindowsServer2016 = types.WindowsServer2016
var WindowsServer2019 = types.WindowsServer2019
var WindowsServer2022 = types.WindowsServer2022
var WindowsServer2025 = types.WindowsServer2025
var WindowsServerDefault = types.WindowsServerDefault
var WindowsServerVersionsForE2E = types.WindowsServerVersionsForE2E
var WindowsSetupSSHScriptContent = types.WindowsSetupSSHScriptContent
var ZypperDisableUnattendedUpgradesScriptContent = types.ZypperDisableUnattendedUpgradesScriptContent

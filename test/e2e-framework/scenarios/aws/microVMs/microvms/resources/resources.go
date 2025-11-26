// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resources

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
)

const (
	SharedFSMount = "sharedFSMount"
	DomainID      = "domainID"
	MACAddress    = "mac"
	DHCPEntries   = "dhcpEntries"
	ImageName     = "imageName"
	VolumeKey     = "volumeKey"
	ImagePath     = "imagePath"
	PoolName      = "poolName"
	PoolPath      = "poolPath"
	Nvram         = "nvram"
	Efi           = "efi"
	Format        = "format"
	VCPU          = "vcpu"
	CPUTune       = "cputune"
	Hypervisor    = "hypervisor"
	CommandLine   = "commandLine"
	DiskDriver    = "diskDriver"
)

const (
	fileConsole = "file"
	ptyConsole  = "pty"
)

const (
	RAMPool     vmconfig.PoolType = "ram"
	DefaultPool vmconfig.PoolType = "default"
)

var consoles = map[string]libvirt.DomainConsoleArgs{
	fileConsole: {
		Type:       pulumi.String("file"),
		TargetPort: pulumi.String("0"),
		TargetType: pulumi.String("serial"),
	},
	ptyConsole: {
		Type:       pulumi.String("pty"),
		TargetPort: pulumi.String("0"),
		TargetType: pulumi.String("serial"),
	},
}

var kernelCmdlines = []map[string]string{
	{"acpi": "off"},
	{"panic": "-1"},
	{"root": "/dev/vda"},
	{"net.ifnames": "0"},
	{"_": "rw"},
}

type ResourceCollection interface {
	GetDomainXLS(args map[string]pulumi.StringInput) pulumi.StringOutput
	GetVolumeXML(*RecipeLibvirtVolumeArgs) pulumi.StringOutput
	GetPoolXML(args map[string]pulumi.StringInput) pulumi.StringOutput
	GetLibvirtDomainArgs(*RecipeLibvirtDomainArgs) (*libvirt.DomainArgs, error)
}

type DiskTarget string

type DomainDisk struct {
	VolumeID   pulumi.StringPtrInput
	Target     string
	Mountpoint string
}

type RecipeLibvirtDomainArgs struct {
	DomainName        string
	Vcpu              int
	Memory            int
	Xls               pulumi.StringOutput
	KernelPath        string
	Disks             []DomainDisk
	Resources         ResourceCollection
	ExtraKernelParams map[string]string
	Machine           string
	ConsoleType       string
	Type              string
}

type RecipeLibvirtVolumeArgs struct {
	PoolType vmconfig.PoolType
	XMLArgs  map[string]pulumi.StringInput
}

func GetConsolePath(domainName string) string {
	return fmt.Sprintf("/tmp/ddvm-%s.log", domainName)
}

func setupConsole(consoleType, domainName string) (libvirt.DomainConsoleArgs, error) {
	if consoleType == fileConsole {
		console := consoles[consoleType]
		console.SourcePath = pulumi.String(GetConsolePath(domainName))
		return console, nil
	}

	// default console type is `pty`
	return consoles[ptyConsole], nil
}

func formatResourceXML(xml string, args map[string]pulumi.StringInput) pulumi.StringOutput {
	var templateArgsPromise []interface{}

	// The Replacer functionality expects a list in the format
	// `{placeholder} val` as input for formatting a piece of text
	for k, v := range args {
		templateArgsPromise = append(templateArgsPromise, pulumi.Sprintf("{%s}", k), v)
	}

	pulumiXML := pulumi.All(templateArgsPromise...).ApplyT(func(promises []interface{}) (string, error) {
		var templateArgs []string

		for _, promise := range promises {
			templateArgs = append(templateArgs, promise.(string))
		}

		r := strings.NewReplacer(templateArgs...)
		return r.Replace(xml), nil
	}).(pulumi.StringOutput)

	return pulumiXML
}

func isLocalRecipe(recipe string) bool {
	return (recipe == vmconfig.RecipeCustomLocal) || (recipe == vmconfig.RecipeDistroLocal)
}

func GetLocalArchRecipe(recipe string) string {
	var prefix string

	if !isLocalRecipe(recipe) {
		return recipe
	}

	if strings.HasPrefix(recipe, "distro") {
		prefix = "distro"
	} else if strings.HasPrefix(recipe, "custom") {
		prefix = "custom"
	} else {
		panic("unknown recipe " + recipe)
	}

	if runtime.GOARCH == "amd64" {
		return fmt.Sprintf("%s-x86_64", prefix)
	} else if runtime.GOARCH == "arm64" {
		return fmt.Sprintf("%s-arm64", prefix)
	}

	panic("unknown recipe " + recipe)
}

func NewResourceCollection(recipe string) ResourceCollection {
	archSpecificRecipe := GetLocalArchRecipe(recipe)

	switch archSpecificRecipe {
	case vmconfig.RecipeCustomARM64:
		return NewARM64ResourceCollection(recipe)
	case vmconfig.RecipeCustomAMD64:
		return NewAMD64ResourceCollection(recipe)
	case vmconfig.RecipeDistroARM64:
		return NewDistroARM64ResourceCollection(recipe)
	case vmconfig.RecipeDistroAMD64:
		return NewDistroAMD64ResourceCollection(recipe)
	case vmconfig.RecipeDefault:
		return NewDefaultResourceCollection(recipe)
	default:
		panic("unknown recipe: " + archSpecificRecipe)
	}
}

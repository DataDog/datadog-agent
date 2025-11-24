// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resources

import (
	// import embed
	_ "embed"
	"fmt"
	"sort"

	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed distro/domain-amd64.xsl
var distroDomainXLS string

//go:embed distro/domain-arm64.xsl
var distroARM64DomainXLS string

type DistroAMD64ResourceCollection struct {
	recipe string
}

func NewDistroAMD64ResourceCollection(recipe string) *DistroAMD64ResourceCollection {
	return &DistroAMD64ResourceCollection{
		recipe: recipe,
	}
}

func (a *DistroAMD64ResourceCollection) GetDomainXLS(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return formatResourceXML(distroDomainXLS, args)
}

func (a *DistroAMD64ResourceCollection) GetVolumeXML(args *RecipeLibvirtVolumeArgs) pulumi.StringOutput {
	return GetDefaultVolumeXML(args, a.recipe)
}

func (a *DistroAMD64ResourceCollection) GetPoolXML(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return GetDefaultPoolXML(args, a.recipe)
}

func (a *DistroAMD64ResourceCollection) GetLibvirtDomainArgs(args *RecipeLibvirtDomainArgs) (*libvirt.DomainArgs, error) {
	var disks libvirt.DomainDiskArray
	sort.Slice(args.Disks, func(i, j int) bool {
		return args.Disks[i].Target < args.Disks[j].Target
	})
	for _, disk := range args.Disks {
		disks = append(disks, libvirt.DomainDiskArgs{
			VolumeId: disk.VolumeID,
		})
	}

	console, err := setupConsole(args.ConsoleType, args.DomainName)
	if err != nil {
		return nil, fmt.Errorf("failed to setup console for domain %s: %v", args.DomainName, err)
	}

	domainArgs := libvirt.DomainArgs{
		Name: pulumi.String(args.DomainName),
		Consoles: libvirt.DomainConsoleArray{
			console,
		},
		Disks:  disks,
		Memory: pulumi.Int(args.Memory),
		Vcpu:   pulumi.Int(args.Vcpu),
		Xml: libvirt.DomainXmlArgs{
			Xslt: args.Xls,
		},
	}

	if args.Machine != "" {
		domainArgs.Machine = pulumi.String(args.Machine)
	}
	if args.Type != "" {
		domainArgs.Type = pulumi.String(args.Type)
	}

	return &domainArgs, nil
}

type DistroARM64ResourceCollection struct {
	recipe string
}

func NewDistroARM64ResourceCollection(recipe string) *DistroARM64ResourceCollection {
	return &DistroARM64ResourceCollection{
		recipe: recipe,
	}
}

func (a *DistroARM64ResourceCollection) GetDomainXLS(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return formatResourceXML(distroARM64DomainXLS, args)
}

func (a *DistroARM64ResourceCollection) GetVolumeXML(args *RecipeLibvirtVolumeArgs) pulumi.StringOutput {
	return GetDefaultVolumeXML(args, a.recipe)
}

func (a *DistroARM64ResourceCollection) GetPoolXML(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return GetDefaultPoolXML(args, a.recipe)
}

func (a *DistroARM64ResourceCollection) GetLibvirtDomainArgs(args *RecipeLibvirtDomainArgs) (*libvirt.DomainArgs, error) {
	var disks libvirt.DomainDiskArray

	sort.Slice(args.Disks, func(i, j int) bool {
		return args.Disks[i].Target < args.Disks[j].Target
	})
	for _, disk := range args.Disks {
		disks = append(disks, libvirt.DomainDiskArgs{
			VolumeId: disk.VolumeID,
		})
	}

	console, err := setupConsole(args.ConsoleType, args.DomainName)
	if err != nil {
		return nil, fmt.Errorf("failed to setup console for domain %s: %v", args.DomainName, err)
	}

	domainArgs := libvirt.DomainArgs{
		Name: pulumi.String(args.DomainName),
		Consoles: libvirt.DomainConsoleArray{
			console,
		},
		Disks:   disks,
		Memory:  pulumi.Int(args.Memory),
		Vcpu:    pulumi.Int(args.Vcpu),
		Machine: pulumi.String("virt"),
		Xml: libvirt.DomainXmlArgs{
			Xslt: args.Xls,
		},
	}

	if args.Machine != "" {
		domainArgs.Machine = pulumi.String(args.Machine)
	}
	if args.Type != "" {
		domainArgs.Type = pulumi.String(args.Type)
	}

	return &domainArgs, nil
}

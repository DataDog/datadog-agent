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

//go:embed amd64/domain.xsl
var amd64DomainXLS string

type AMD64ResourceCollection struct {
	recipe string
}

func NewAMD64ResourceCollection(recipe string) *AMD64ResourceCollection {
	return &AMD64ResourceCollection{
		recipe: recipe,
	}
}

func (a *AMD64ResourceCollection) GetDomainXLS(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return formatResourceXML(amd64DomainXLS, args)
}

func (a *AMD64ResourceCollection) GetVolumeXML(args *RecipeLibvirtVolumeArgs) pulumi.StringOutput {
	return GetDefaultVolumeXML(args, a.recipe)
}

func (a *AMD64ResourceCollection) GetPoolXML(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return GetDefaultPoolXML(args, a.recipe)
}

func (a *AMD64ResourceCollection) GetLibvirtDomainArgs(args *RecipeLibvirtDomainArgs) (*libvirt.DomainArgs, error) {
	var cmdlines []map[string]string
	for cmd, val := range args.ExtraKernelParams {
		cmdlines = append(cmdlines, map[string]string{cmd: val})
	}
	cmdlines = append(cmdlines, kernelCmdlines...)

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
		Disks:    disks,
		Kernel:   pulumi.String(args.KernelPath),
		Cmdlines: pulumi.ToStringMapArray(cmdlines),
		Memory:   pulumi.Int(args.Memory),
		Vcpu:     pulumi.Int(args.Vcpu),
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

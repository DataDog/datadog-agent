// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package resources

import (
	// import embed
	_ "embed"

	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
)

//go:embed default/domain.xsl
var defaultDomainXLS string

//go:embed default/network.xsl
var defaultNetworkXLS string

//go:embed default/pool.xml
var defaultPoolXML string

//go:embed default/volume.xml
var defaultVolumeXML string

//go:embed default/raw_volume.xml
var defaultRawVolumeXML string

//go:embed default/volume_local.xsl
var defaultLocalVolumeXLS string

var remoteVolumeXMLs = map[vmconfig.PoolType]string{
	DefaultPool: defaultVolumeXML,
	RAMPool:     defaultRawVolumeXML,
}

func GetDefaultDomainXLS(...interface{}) string {
	return defaultDomainXLS
}

func GetDefaultNetworkXLS(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return formatResourceXML(defaultNetworkXLS, args)
}

func GetDefaultVolumeXML(args *RecipeLibvirtVolumeArgs, recipe string) pulumi.StringOutput {
	if isLocalRecipe(recipe) {
		return formatResourceXML(defaultLocalVolumeXLS, args.XMLArgs)
	}

	return formatResourceXML(remoteVolumeXMLs[args.PoolType], args.XMLArgs)
}

func GetDefaultPoolXML(args map[string]pulumi.StringInput, _ string) pulumi.StringOutput {
	return formatResourceXML(defaultPoolXML, args)
}

type DefaultResourceCollection struct {
	recipe string
}

func NewDefaultResourceCollection(recipe string) *DefaultResourceCollection {
	return &DefaultResourceCollection{
		recipe: recipe,
	}
}

func (a *DefaultResourceCollection) GetDomainXLS(_ map[string]pulumi.StringInput) pulumi.StringOutput {
	return pulumi.Sprintf("%s", GetDefaultDomainXLS())
}

func (a *DefaultResourceCollection) GetVolumeXML(args *RecipeLibvirtVolumeArgs) pulumi.StringOutput {
	return GetDefaultVolumeXML(args, a.recipe)
}

func (a *DefaultResourceCollection) GetPoolXML(args map[string]pulumi.StringInput) pulumi.StringOutput {
	return GetDefaultPoolXML(args, a.recipe)
}

func (a *DefaultResourceCollection) GetLibvirtDomainArgs(_ *RecipeLibvirtDomainArgs) (*libvirt.DomainArgs, error) {
	return nil, nil
}

package fakeintake

import (
	"github.com/DataDog/test-infra-definitions/components"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type FakeintakeOutput struct { // nolint:revive, We want to keep the name as <Component>Output
	components.JSONImporter

	Host   string `json:"host"`
	Scheme string `json:"scheme"`
	Port   uint32 `json:"port"`
	URL    string `json:"url"`
}

type Fakeintake struct {
	pulumi.ResourceState
	components.Component

	Host   pulumi.StringOutput `pulumi:"host"`
	Scheme pulumi.StringOutput `pulumi:"scheme"` // Scheme is a string as it's known in code and is useful to check HTTP/HTTPS
	Port   pulumi.IntOutput    `pulumi:"port"`   // Same for Port

	URL pulumi.StringOutput `pulumi:"url"`
}

func (fi *Fakeintake) Export(ctx *pulumi.Context, out *FakeintakeOutput) error {
	return components.Export(ctx, fi, out)
}

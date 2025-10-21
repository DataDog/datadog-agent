package compute

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-azure-native-sdk/compute/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func parseImageReferenceURN(urn string) (compute.ImageReferencePtrInput, error) {
	splitted := strings.Split(urn, imageURNSeparator)
	if len(splitted) != 4 {
		return nil, fmt.Errorf("unable to parse image: %s", urn)
	}

	return compute.ImageReferenceArgs{
		Publisher: pulumi.StringPtr(splitted[0]),
		Offer:     pulumi.StringPtr(splitted[1]),
		Sku:       pulumi.StringPtr(splitted[2]),
		Version:   pulumi.StringPtr(splitted[3]),
	}, nil
}

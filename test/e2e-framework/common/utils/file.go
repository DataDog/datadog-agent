package utils

import (
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func ReadSecretFile(filePath string) (pulumi.StringOutput, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return pulumi.StringOutput{}, err
	}

	s := pulumi.ToSecret(pulumi.String(string(b))).(pulumi.StringOutput)

	return s, nil
}

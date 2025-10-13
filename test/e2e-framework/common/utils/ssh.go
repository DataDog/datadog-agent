package utils

import (
	"fmt"
	"os"
	"path"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	defaultPublicKeyFilePath = ".ssh/id_rsa.pub"
)

func GetSSHPublicKey(filePath string) (pulumi.StringOutput, error) {
	if filePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return pulumi.StringOutput{}, fmt.Errorf("unable to read SSH key, err: %v", err)
		}

		filePath = path.Join(homeDir, defaultPublicKeyFilePath)
	}

	return ReadSecretFile(filePath)
}

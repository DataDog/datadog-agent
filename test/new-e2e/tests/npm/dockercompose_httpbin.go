package npm

import (
	_ "embed"

	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed "config/dockercompose_httpbin.yaml"
var dockerHTTPBinComposeYaml string

func dockerHTTPBinCompose() docker.ComposeInlineManifest {
	return docker.ComposeInlineManifest{
		Name:    "httpbin",
		Content: pulumi.String(dockerHTTPBinComposeYaml),
	}
}

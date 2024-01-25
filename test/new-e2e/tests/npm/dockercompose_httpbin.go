package npm

import (
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v2"
)

func dockerHTTPBinCompose() docker.ComposeInlineManifest {
	httpbinManifestContent := pulumi.All().ApplyT(func(args []interface{}) (string, error) {
		agentManifest := docker.ComposeManifest{
			Version: "3.9",
			Services: map[string]docker.ComposeManifestService{
				"httpbin": {
					Privileged:    true,
					Image:         "mccutchen/go-httpbin",
					ContainerName: "httpbin",
					Pid:           "host",
					Ports:         []string{"80:8080/tcp"},
				},
			},
		}
		data, err := yaml.Marshal(agentManifest)
		return string(data), err
	}).(pulumi.StringOutput)

	return docker.ComposeInlineManifest{
		Name:    "httpbin",
		Content: httpbinManifestContent,
	}
}

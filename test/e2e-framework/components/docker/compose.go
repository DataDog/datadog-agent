package docker

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

type ComposeInlineManifest struct {
	Name    string
	Content pulumi.StringInput
}

type ComposeManifest struct {
	Version  string                            `yaml:"version"`
	Services map[string]ComposeManifestService `yaml:"services"`
}

type ComposeManifestService struct {
	Pid           string         `yaml:"pid,omitempty"`
	Privileged    bool           `yaml:"privileged,omitempty"`
	Ports         []string       `yaml:"ports,omitempty"`
	Image         string         `yaml:"image"`
	ContainerName string         `yaml:"container_name"`
	Volumes       []string       `yaml:"volumes"`
	Environment   map[string]any `yaml:"environment"`
}

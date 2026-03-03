// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package vmconfig

type PoolType string

type VMSetID string

func (id VMSetID) String() string {
	return string(id)
}

const (
	RecipeCustomAMD64 = "custom-x86_64"
	RecipeCustomARM64 = "custom-arm64"
	RecipeDistroAMD64 = "distro-x86_64"
	RecipeDistroARM64 = "distro-arm64"
	RecipeCustomLocal = "custom-local"
	RecipeDistroLocal = "distro-local"
	RecipeDefault     = "default"
)

type Disk struct {
	Type         PoolType `json:"type"`
	BackingStore string   `json:"source"`
	Target       string   `json:"target"`
	Size         string   `json:"size,omitempty"`
	Mountpoint   string   `json:"mount_point"`
}

type Kernel struct {
	Dir         string            `json:"dir"`
	Tag         string            `json:"tag"`
	ImageSource string            `json:"image_source,omitempty"`
	ExtraParams map[string]string `json:"extra_params,omitempty"`
}

type Image struct {
	ImageName      string `json:"image_path,omitempty"`
	ImageSourceURI string `json:"image_source,omitempty"`
}

type Host struct {
	AvailableCPUs int `json:"available_cpus"`
}

type VMSet struct {
	Tags        []string `json:"tags"`
	Recipe      string   `json:"recipe"`
	Kernels     []Kernel `json:"kernels"`
	VCpu        []int    `json:"vcpu"`
	Memory      []int    `json:"memory"`
	Img         Image    `json:"image"`
	Machine     string   `json:"machine,omitempty"`
	Arch        string
	ID          VMSetID `json:"omitempty"`
	Disks       []Disk  `json:"disks,omitempty"`
	ConsoleType string  `json:"console_type"`
	VMHost      Host    `json:"host,omitempty"`
}

type Config struct {
	Workdir string  `json:"workdir"`
	VMSets  []VMSet `json:"vmsets"`
	SSHKey  string  `json:"sshkey,omitempty"`
	SSHUser string  `json:"ssh_user,omitempty"`
}

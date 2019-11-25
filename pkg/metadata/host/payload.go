// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package host

type systemStats struct {
	CPUCores  int32     `json:"cpuCores"`
	Machine   string    `json:"machine"`
	Platform  string    `json:"platform"`
	Pythonv   string    `json:"pythonV"`
	Processor string    `json:"processor"`
	Macver    osVersion `json:"macV"`
	Nixver    osVersion `json:"nixV"`
	Fbsdver   osVersion `json:"fbsdV"`
	Winver    osVersion `json:"winV"`
}

// Meta is the metadata nested under the meta key
type Meta struct {
	SocketHostname string   `json:"socket-hostname"`
	Timezones      []string `json:"timezones"`
	SocketFqdn     string   `json:"socket-fqdn"`
	EC2Hostname    string   `json:"ec2-hostname"`
	Hostname       string   `json:"hostname"`
	HostAliases    []string `json:"host_aliases"`
	InstanceID     string   `json:"instance-id"`
	AgentHostname  string   `json:"agent-hostname"`
}

// NetworkMeta is metadata about the host's network
type NetworkMeta struct {
	ID string `json:"network-id"`
}

type tags struct {
	System              []string `json:"system,omitempty"`
	GoogleCloudPlatform []string `json:"google cloud platform,omitempty"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Os            string            `json:"os"`
	PythonVersion string            `json:"python"`
	SystemStats   *systemStats      `json:"systemStats"`
	Meta          *Meta             `json:"meta"`
	HostTags      *tags             `json:"host-tags"`
	ContainerMeta map[string]string `json:"container-meta,omitempty"`
	NetworkMeta   *NetworkMeta      `json:"network"`
}

package api

import (
	"context"
	"net/http"
)

type Component interface {
	PodContainerMetadata(context.Context, MetadataRequest) (MetadataResponse, error)
	PodContainerMetadataHandlerFunc() http.HandlerFunc
}

type MetadataRequest struct {
	PodName        string
	PodNamespace   string
	InitContainers map[string]ContainerSpec
}

type ContainerMetadata struct {
	Name       string   `json:"name,omitempty"`
	Cmd        []string `json:"cmd,omitempty"`
	WorkingDir string   `json:"workingDir,omitempty"`

	// N.B. We can add language and process data here as well.
}

type MetadataResponse struct {
	Containers map[string]ContainerMetadata `json:"containers"`
}

// ContainerSpec refers to the data we are passing from the _original_
// kubernetes container spec to our container inspection code.
//
// Note that in the actual runtime, this will have already been changed
// by mutating webhooks-- we need to know what it was before anything
// happened.
//
// This data will be forwarded from the webhook, to a base64 json encoded
// argument in MetadataRequest.Request, then through the container inspection
// package.
type ContainerSpec struct {
	Name       string   `json:"name,omitEmpty"`
	Command    []string `json:"command,omitempty"`
	Args       []string `json:"args,omitempty"`
	WorkingDir string   `json:"workingDir,omitempty"`
	Image      string   `json:"image,omitempty"`
}

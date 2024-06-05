package api

import (
	"context"
	"net/http"
	"time"
)

type Component interface {
	PodContainerMetadata(context.Context, MetadataRequest) (MetadataResponse, error)
	PodContainerMetadataHandlerFunc() http.HandlerFunc
}

type MetadataRequest struct {
	// PodName is the name of the pod we are looking for.
	PodName        string

	// PodNamespace is the namespace of the given pod.
	PodNamespace   string

	// InitContainers are the containers we are looking for.
	InitContainers map[string]ContainerSpec

	// StaleImageDuration refers to how old image Metadata can be in
	// our client/collector.
	//
	// If the image data is older than StaleImageDuration,
	// we can assume we haven't gotten the data yet since we pull
	// run a _new_ container for the image we care about _right before_
	// we get the request.
	StaleImageDuration *time.Duration
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"strings"
)

// Image is a type to manage container images. Use NewImage to create an image from a library string.
type Image struct {
	// Registry is the container registry for an image, formatted the same way the cluster agent expects a registry.
	// Ex: gcr.io/datadoghq
	Registry string
	// Name is the name of the image.
	// Ex: dd-lib-java-init
	Name string
	// Tag is the version of the image.
	// Ex: v1
	Tag string
}

// NewImage takes a source image string from a pod spec and returns an struct representation. An example source image
// looks like: gcr.io/datadoghq/dd-lib-java-init:v1.
func NewImage(source string) (*Image, error) {
	// Split the string on the colon to get the image and tag: gcr.io/datadoghq/dd-lib-java-init:v1
	parts := strings.Split(source, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("could not determine image tag from %s", source)
	}
	fullImage := parts[0]
	tag := parts[1]

	// Split the image to get the last item and the registry: gcr.io/datadoghq/dd-lib-java-init
	parts = strings.Split(fullImage, "/")
	if len(parts) < 1 {
		return nil, fmt.Errorf("could not determine registry and name from %s", fullImage)
	}
	name := parts[len(parts)-1]
	registry := strings.TrimSuffix(fullImage, "/"+name)

	// Return the parsed image.
	return &Image{
		Registry: registry,
		Name:     name,
		Tag:      tag,
	}, nil
}

// String implements the fmt.Stringer interface to provide a fully qualified image to use in a pod spec.
func (i *Image) String() string {
	return i.Registry + "/" + i.Name + ":" + i.Tag
}

// ToLibrary converts an image to a library if the language is valid. This will error for the injector image.
func (i *Image) ToLibrary() (*Library, error) {
	lang, err := ExtractLibraryLanguage(i.Name)
	if err != nil {
		return nil, fmt.Errorf("could not extract library language: %w", err)
	}
	return NewLibrary(lang, i.Tag), nil
}

func (i *Image) LibInfo(ctrName string) libInfo {
	return libInfo{
		ctrName:    ctrName,
		image:      i.String(),
		registry:   i.Registry,
		repository: i.Name,
		tag:        i.Tag,
	}
}

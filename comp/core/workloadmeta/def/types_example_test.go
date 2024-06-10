// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package workloadmeta

import "fmt"

func getAnEntity() Entity {
	return &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "abc123",
		},
		Image: ContainerImage{
			Name: "cassandra",
		},
	}
}

func ExampleEntity() {
	entity := getAnEntity()

	if container, ok := entity.(*Container); ok {
		fmt.Printf("Got container with image %s\n", container.Image.Name)
	} else {
		fmt.Printf("Not a Container")
	}

	// Output: Got container with image cassandra
}

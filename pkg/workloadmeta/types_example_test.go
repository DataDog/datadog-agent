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

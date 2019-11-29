package topologycollectors

/*
// publish container correlation
	containerToNodeCorrelationChannel := make(chan *ContainerToNodeCorrelation)
	go func() {
		for _, cnc := range []*ContainerToNodeCorrelation{
			{
				NodeName: "test-node-3",
				MappingFunction: func(nodeIdentifier string) (components []*topology.Component, relations []*topology.Relation) {

					containerExternalID := ic.buildContainerExternalID("test-pod", "test-container")
					podExternalID := ic.buildPodExternalID("test-pod")

					components = append(components, &topology.Component{
						ExternalID: containerExternalID,
						Type: topology.Type{ Name: "container"},
						Data: topology.Data{
							"name": "test-container",
							"pod": "test-pod",
							"podIP": "10.20.30.40",
							"namespace": "test-namespace",
						},
					})

					relations = append(relations, &topology.Relation{
						ExternalID: fmt.Sprintf("%s->%s", containerExternalID, podExternalID),
						Type: topology.Type{ Name: "enclosed_in"},
						SourceID: containerExternalID,
						TargetID: podExternalID,
						Data: topology.Data{},
					})

					return components, relations
				},
			},
		} {
			containerToNodeCorrelationChannel <- cnc
		}

		close(containerToNodeCorrelationChannel)
	}()
*/

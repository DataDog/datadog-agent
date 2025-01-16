package workloadmeta

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func getAContainerEntity(id, image string) Entity {
	return &Container{
		EntityID: EntityID{
			ID:   id,
			Kind: KindContainer,
		},
		EntityMeta: EntityMeta{
			Labels: map[string]string{
				"com.datadoghq.ad.check_names":  "[\"apache\"]",
				"com.datadoghq.ad.init_configs": "[{}]",
				"com.datadoghq.ad.instances":    "[{\"apache_status_url\": \"http://%%host%%/server-status?auto\"}]",
			},
		},
		Runtime: ContainerRuntimeDocker,
		Image: ContainerImage{
			Name: image,
		},
	}
}

func getAContainerState() ContainerState {
	return ContainerState{
		CreatedAt:  time.Time{},
		StartedAt:  time.Time{},
		FinishedAt: time.Time{},
		ExitCode:   pointer.Ptr(int64(100)),
		Health:     ContainerHealthHealthy,
	}
}

func getAPodEntity(id string) Entity {
	return &KubernetesPod{
		EntityMeta: EntityMeta{
			Annotations: map[string]string{
				"ad.datadoghq.com/apache.checks": `{
					"http_check": {
						"instances": [
							{
								"name": "My service",
								"url": "http://%%host%%",
								"timeout": 1
							}
						]
					}
				}`,
				"ad.datadoghq.com/apache.check_names":  "[\"invalid\"]",
				"ad.datadoghq.com/apache.init_configs": "[{}]",
				"ad.datadoghq.com/apache.instances":    "[{}]",
			},
		},
		EntityID: EntityID{
			ID: id,
		},

		Containers: []OrchestratorContainer{
			{
				Name: "abc",
				ID:   "3b8efe0c50e1",
			},
		},
	}
}

func getAProcessEntity() Entity {
	return &Process{
		EntityID: EntityID{
			ID:   "123",
			Kind: KindProcess,
		},
	}
}

func BenchmarkProcessCollectorEvent(b *testing.B) {
	containerEntity := getAContainerEntity("3b8efe0c50e1", "cassandra")
	podEntity := getAPodEntity("3b8efe0c50e1")
	processEntity := getAProcessEntity()
	events := []CollectorEvent{
		{
			Type:   EventType(0),
			Source: "container",
			Entity: containerEntity,
		},
		{
			Type:   EventType(1),
			Source: "kubernetes",
			Entity: podEntity,
		},
		{
			Type:   EventType(2),
			Source: "processes",
			Entity: processEntity,
		},
	}

	for _, event := range events {
		b.Run("Run collector event bench test on "+string(event.Source), func(b *testing.B) {
			b.ReportAllocs() // Report memory allocations
			b.ResetTimer()
			// process events
			for i := 0; i < b.N; i++ {
				size := 1000
				events := make([]CollectorEvent, size)
				for i := 0; i < size; i++ {
					events[i] = event
				}
				// Simulate some processing
				for i := 0; i < size; i++ {
					_ = events[i]
				}
			}
		})
	}
}

func BenchmarkProcessEntity(b *testing.B) {
	containerEntity := getAContainerEntity("3b8efe0c50e1", "cassandra")
	podEntity := getAPodEntity("3b8efe0c50e1")
	procEntity := getAProcessEntity()
	entities := []Entity{containerEntity, podEntity, procEntity}

	for _, entity := range entities {
		b.Run("Run process entity bench test on "+string(entity.GetID().Kind), func(b *testing.B) {
			b.ReportAllocs() // Report memory allocations
			b.ResetTimer()
			// process entities
			for i := 0; i < b.N; i++ {
				size := 10
				entities := make([]Entity, size)
				for i := 0; i < size; i++ {
					entities[i] = entity.DeepCopy()
				}
				// Simulate some processing
				for i := 0; i < size; i++ {
					_ = entities[i]
				}
			}
		})
	}
}

func BenchmarkProcessContainerState(b *testing.B) {
	b.Run("Run process container state bench test", func(b *testing.B) {
		b.ReportAllocs() // Report memory allocations
		b.ResetTimer()
		// process container states
		for i := 0; i < b.N; i++ {
			size := 1000
			states := make([]ContainerState, size)
			for i := 0; i < size; i++ {
				states[i] = getAContainerState()
			}
			// Simulate some processing
			var sum int64 = 0
			for i := 0; i < size; i++ {
				sum += *(states[i].ExitCode)
			}
		}
	})
}

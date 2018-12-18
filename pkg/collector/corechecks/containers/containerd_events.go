// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build containerd

package containers

import (
	"time"
	"github.com/containerd/containerd/namespaces"
	"fmt"
	containerd2 "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"context"
)

//ContainerType contains the timestamp to make sure we flush all events that happened between two checks
type ContainerEvent struct {
	ContainerID   string
	Timestamp     time.Time
	Topic         string
	Namespace     string
}

type Subscriber struct {
	Name        string
	Filters     []string
	Events      []ContainerEvent
	Namespaces   []string
}

func createEventSubscriber(name string) *Subscriber {
	return &Subscriber{
		Name:name,
	}
}

// Run should only be called once, at start time.
func (s *Subscriber) Run() error {
	ctrItf, err := containerd2.GetContainerdUtil()
	if err != nil {
		return err
	}

	ctx := context.Background()
	ctx = namespaces.WithNamespace(ctx,"k8s.io")
	ev := ctrItf.GetEvents()

	//filters := []string{
	//	`topic=="/tasks/exit"`,
	//	`topic=="/containers/delete"`,
	//}

	stream, _ := ev.Subscribe(ctx, s.Filters...)
	for {
		message := <- stream
		//val := 	string(message.Event.Value)

		fmt.Printf("ns %s, ts %#v, topic is %s, typeurl %s \n",
			message.Namespace,
			message.Timestamp,
			message.Topic,
			message.Event.TypeUrl)

		s.Events = append(s.Events, ContainerEvent{
			ContainerID: "foo",
		})

		fmt.Printf("val is %s \n", string(message.Event.GetValue()))
	}
	return nil
}

// flush should be called every time you want to
func (s *Subscriber) Flush(timestamp int64) []ContainerEvent {
	if timestamp > s.Events[0].Timestamp.Unix() {
		return s.Events
	}
	return nil
}

// convertevents
func convertEvents(ev []ContainerEvent)  {
	// process ev into Datadog ev and returns them to be submitted.
	return

}

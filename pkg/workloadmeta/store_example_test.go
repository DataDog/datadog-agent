// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package workloadmeta

import "fmt"

func init() {
	// CreateGlobalStore is usually called by the main cmd in the existing
	// agents, so there's no need to call it yourself
	CreateGlobalStore(nil)
}

func ExampleStore_Subscribe() {
	filter := NewFilter([]Kind{KindContainer}, SourceRuntime, EventTypeAll)
	ch := GetGlobalStore().Subscribe("test", NormalPriority, filter)

	go func() {
		for bundle := range ch {
			// close Ch to indicate that the Store can proceed to the next subscriber
			close(bundle.Ch)

			for _, evt := range bundle.Events {
				if evt.Type == EventTypeSet {
					fmt.Printf("new/updated container: %s\n", evt.Entity.GetID().ID)
				} else {
					fmt.Printf("container removed: %s\n", evt.Entity.GetID().ID)
				}
			}
		}
	}()

	// unsubscribe immediately so that this example does not hang
	GetGlobalStore().Unsubscribe(ch)

	// Output:
}

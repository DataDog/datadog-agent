package workloadmeta

import "fmt"

func ExampleStore_Subscribe() {
	filter := NewFilter([]Kind{KindContainer}, SourceRuntime)
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

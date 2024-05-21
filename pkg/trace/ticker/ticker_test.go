package ticker

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
)

type testTicker struct {
	c chan struct{}
}

func (t *testTicker) Tasks() []TickTask {
	return []TickTask{
		{
			Interval: 50 * time.Millisecond,
			Task: func() {
				t.c <- struct{}{}
			},
		},
		{
			Interval: 10 * time.Millisecond,
			Task: func() {
				t.c <- struct{}{}
			},
		},
	}
}

func TestRunTickers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	outchan := make(chan struct{}, 1000)
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
		close(outchan)
	}()

	start := time.Now()
	RunTickers(ctx, &statsd.NoOpClient{}, []Ticker{
		&testTicker{c: outchan},
	})

	for i := 0; i < 10; i++ {
		_, ok := <-outchan
		if !ok {
			t.Fatalf("Expected to receive 10 elements within 500 milliseconds, but only got %d before the channel closed.", i)
		}
	}

	if time.Now().Add(-100 * time.Millisecond).Before(start) {
		t.Fatal("Expected to receive 10 elements within 500 milliseconds")
	}
}

package ticker

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// A ticker is any object that needs to perform some task at
// regular intervals
type Ticker interface {
	Tasks() []TickTask
}

type TickTask struct {
	Interval time.Duration
	Task     func()
}

func RunTickers(ctx context.Context, sd statsd.ClientInterface, ts []Ticker) {
	m := make(map[time.Duration][]func())
	for _, ti := range ts {
		tasks := ti.Tasks()
		for _, t := range tasks {
			m[t.Interval] = append(m[t.Interval], t.Task)
		}
	}

	type ticklist struct {
		t  *time.Ticker
		fs []func()
	}
	var s []ticklist
	var sc []reflect.SelectCase
	for int, fs := range m {
		tl := ticklist{t: time.NewTicker(int), fs: fs}
		s = append(s, tl)
		sc = append(sc, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(tl.t.C)})
	}

	//s = append(s, ticklist{fs: func() {}}
	sc = append(sc, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})

	defer watchdog.LogOnPanic(sd)
	for {
		i, _, ok := reflect.Select(sc)
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			fmt.Printf("Shutting down tickers: %v\n", err)
			return
		}
		fsi := s[i]
		for _, f := range fsi.fs {
			f()
		}
	}

}

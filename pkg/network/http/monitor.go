// +build linux_bpf

package http

import (
	"fmt"
	"time"

	"C"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
)

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Polling a perf buffer that contains notifications about HTTP transaction batches ready to be read;
// * Querying these batches by doing a map lookup;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	batchManager  *batchManager
	perfMap       *manager.PerfMap
	perfHandler   *ddebpf.PerfHandler
	closeFilterFn func()
}

// NewMonitor returns a new Monitor instance
func NewMonitor(procRoot string, m *manager.Manager, h *ddebpf.PerfHandler) (*Monitor, error) {
	filter, _ := m.GetProbe(manager.ProbeIdentificationPair{Section: string(probes.SocketHTTPFilter)})
	if filter == nil {
		return nil, fmt.Errorf("error retrieving socket filter")
	}

	closeFilterFn, err := network.HeadlessSocketFilter(procRoot, filter)
	if err != nil {
		return nil, fmt.Errorf("error enabling HTTP traffic inspection: %s", err)
	}

	batchMap, _, err := m.GetMap(string(probes.HttpBatchesMap))
	if err != nil {
		return nil, err
	}

	batchStateMap, _, err := m.GetMap(string(probes.HttpBatchStateMap))
	if err != nil {
		return nil, err
	}

	notificationMap, _, _ := m.GetMap(string(probes.HttpNotificationsMap))
	numCPUs := int(notificationMap.ABI().MaxEntries)

	pm, found := m.GetPerfMap(string(probes.HttpNotificationsMap))
	if !found {
		return nil, fmt.Errorf("unable to find perf map %s", probes.HttpNotificationsMap)
	}

	return &Monitor{
		batchManager:  newBatchManager(batchMap, batchStateMap, numCPUs),
		perfMap:       pm,
		perfHandler:   h,
		closeFilterFn: closeFilterFn,
	}, nil
}

// Start consuming HTTP events
// Please note the code below is merely an *example* of how to consume the HTTP
// transaction information sent from the eBPF program
func (http *Monitor) Start() error {
	if http == nil {
		return nil
	}

	if err := http.perfMap.Start(); err != nil {
		return fmt.Errorf("error starting perf map: %s", err)
	}

	go func() {
		var (
			misses int
			then   = time.Now()
			hits   = make(map[int]int)
			report = time.NewTicker(30 * time.Second)
		)
		defer report.Stop()

		for {
			select {
			case data, ok := <-http.perfHandler.DataChannel:
				if !ok {
					return
				}

				// The notification we read off the perf ring tells us which HTTP batch of transactions is ready to be read
				notification := toHTTPNotification(data)
				txs := http.batchManager.GetTransactionsFrom(notification)
				aggregate(txs, hits)
			case _, ok := <-http.perfHandler.LostChannel:
				if !ok {
					return
				}

				http.batchManager.misses++
			case now := <-report.C:
				txs := http.batchManager.GetPendingTransactions()
				aggregate(txs, hits)

				delta := float64(now.Sub(then).Seconds())
				log.Infof("http report: 100(%d reqs, %.2f/s) 200(%d reqs, %.2f/s) 300(%d reqs, %.2f/s), 400(%d reqs, %.2f/s) 500(%d reqs, %.2f/s), misses(%d reqs, %.2f/s)",
					hits[100], float64(hits[100])/delta,
					hits[200], float64(hits[200])/delta,
					hits[300], float64(hits[300])/delta,
					hits[400], float64(hits[400])/delta,
					hits[500], float64(hits[500])/delta,
					misses*HTTPBatchSize,
					float64(http.batchManager.misses*HTTPBatchSize)/delta,
				)
				hits = make(map[int]int)
				then = now
				http.batchManager.misses = 0
			}
		}
	}()

	return nil
}

// Stop HTTP monitoring
func (http *Monitor) Stop() {
	if http == nil {
		return
	}

	http.closeFilterFn()
	_ = http.perfMap.Stop(manager.CleanAll)
	http.perfHandler.Stop()
}

// Placeholder code
func aggregate(txs []httpTX, hits map[int]int) {
	for _, tx := range txs {
		hits[tx.StatusClass()]++
	}
}

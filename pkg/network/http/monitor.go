// +build linux_bpf

package http

import (
	"fmt"

	"C"

	"time"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf/manager"
)
import "sync"

// Monitor is responsible for:
// * Creating a raw socket and attaching an eBPF filter to it;
// * Polling a perf buffer that contains notifications about HTTP transaction batches ready to be read;
// * Querying these batches by doing a map lookup;
// * Aggregating and emitting metrics based on the received HTTP transactions;
type Monitor struct {
	batchManager *batchManager
	perfMap      *manager.PerfMap
	perfHandler  *ddebpf.PerfHandler
	telemetry    *telemetry
	pollRequests chan chan struct{}
	statkeeper    *httpStatKeeper

	// termination
	mux           sync.Mutex
	eventLoopWG   sync.WaitGroup
	closeFilterFn func()
	stopped       bool
}

// NewMonitor returns a new Monitor instance
func NewMonitor(m *manager.Manager, h *ddebpf.PerfHandler, closeFilterFn func()) (*Monitor, error) {
	batchMap, _, err := m.GetMap(string(probes.HttpBatchesMap))
	if err != nil {
		return nil, err
	}

	batchStateMap, _, err := mgr.GetMap(string(probes.HttpBatchStateMap))
	if err != nil {
		return nil, err
	}

	notificationMap, _, _ := mgr.GetMap(string(probes.HttpNotificationsMap))
	numCPUs := int(notificationMap.ABI().MaxEntries)

	pm, found := mgr.GetPerfMap(string(probes.HttpNotificationsMap))
	if !found {
		return nil, fmt.Errorf("unable to find perf map %s", probes.HttpNotificationsMap)
	}

	return &Monitor{
		batchManager:  newBatchManager(batchMap, batchStateMap, numCPUs),
		perfMap:       pm,
		perfHandler:   h,
		telemetry:     newTelemetry(),
		pollRequests:  make(chan chan struct{}),
		closeFilterFn: closeFilterFn,
		statkeeper:    newHTTPStatkeeper(),
	}, nil
}

// Start consuming HTTP events
func (m *Monitor) Start() error {
	if m == nil {
		return nil
	}

	if err := m.perfMap.Start(); err != nil {
		return fmt.Errorf("error starting perf map: %s", err)
	}

	m.eventLoopWG.Add(1)
	go func() {
		defer m.eventLoopWG.Done()
		report := time.NewTicker(30 * time.Second)
		defer report.Stop()
		for {
			select {
			case data, ok := <-m.perfHandler.DataChannel:
				if !ok {
					return
				}

				// The notification we read from the perf ring tells us which HTTP batch of transactions is ready to be consumed
				notification := toHTTPNotification(data)
				transactions, err := m.batchManager.GetTransactionsFrom(notification)
				m.process(transactions, err)
			case _, ok := <-m.perfHandler.LostChannel:
				if !ok {
					return
				}

				m.process(nil, errLostBatch)
			case reply, ok := <-m.pollRequests:
				if !ok {
					return
				}

				transactions := m.batchManager.GetPendingTransactions()
				m.process(transactions, nil)
				reply <- struct{}{}
			case <-report.C:
				transactions := m.batchManager.GetPendingTransactions()
				m.process(transactions, nil)
			}
		}
	}()

	return nil
}

// Sync HTTP data between userspace and kernel space
func (m *Monitor) Sync() {
	if m == nil {
		return
	}

	m.mux.Lock()
	defer m.mux.Unlock()
	if m.stopped {
		return
	}

	reply := make(chan struct{}, 1)
	defer close(reply)
	m.pollRequests <- reply
	<-reply
}

// GetHTTPStats returns a map of HTTP stats stored in the following format:
// [source, dest tuple] -> [request path] -> RequestStats object
func (m *Monitor) GetHTTPStats() map[Key]map[string]RequestStats {
	m.Sync()

	if m.statkeeper == nil {
		return nil
	}
	return m.statkeeper.GetAndResetAllStats()
}

func (m *Monitor) GetStats() map[string]interface{} {
	currentTime, telemetryData := m.telemetry.getStats()
	return map[string]interface{}{
		"current_time": currentTime,
		"telemetry":    telemetryData,
	}
}

// Stop HTTP monitoring
func (m *Monitor) Stop() {
	if m == nil {
		return
	}

	m.mux.Lock()
	defer m.mux.Unlock()
	if m.stopped {
		return
	}

	m.closeFilterFn()
	_ = m.perfMap.Stop(manager.CleanAll)
	m.perfHandler.Stop()
	close(m.pollRequests)
	m.eventLoopWG.Wait()
	if http.statkeeper != nil {
		http.statkeeper.Close()
	}
	m.stopped = true
}

func (m *Monitor) process(transactions []httpTX, err error) {
	m.telemetry.aggregate(transactions, err)

	if m.statkeeper != nil && len(transactions) > 0 {
		m.statkeeper.Process(transactions)
	}
}

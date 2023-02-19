// // Unless explicitly stated otherwise all files in this repository are licensed
// // under the Apache License Version 2.0.
// // This product includes software developed at Datadog (https://www.datadoghq.com/).
// // Copyright 2016-present Datadog, Inc.
//
// //go:build linux_bpf
// // +build linux_bpf
package kafka

//
//import (
//	"unsafe"
//
//	"github.com/DataDog/datadog-agent/pkg/network/config"
//	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
//	"github.com/DataDog/datadog-agent/pkg/process/monitor"
//)
//
//// Monitor is responsible for:
//// * Creating a raw socket and attaching an eBPF filter to it;
//// * Polling a perf buffer that contains notifications about Kafka transaction batches ready to be read;
//// * Querying these batches by doing a map lookup;
//// * Aggregating and emitting metrics based on the received Kafka transactions;
//type Monitor struct {
//	consumer *events.Consumer
//	//ebpfProgram    *ebpfProgram
//	telemetry      *Telemetry
//	statkeeper     *KafkaStatKeeper
//	processMonitor *monitor.ProcessMonitor
//
//	// termination
//	closeFilterFn func()
//}
//
//// NewMonitor returns a new Monitor instance
//func NewMonitor(c *config.Config, ebpfProgram *EbpfProgram) (*Monitor, error) {
//	//mgr, err := newEBPFProgram(c, bpfTelemetry)
//	//if err != nil {
//	//	return nil, fmt.Errorf("error setting up kafka ebpf program: %s", err)
//	//}
//
//	//if err := mgr.Init(); err != nil {
//	//	err2 := errors.Unwrap(err)
//	//	err3, ok := errors.Unwrap(err2).(*ebpf.VerifierError)
//	//	if ok {
//	//		for _, l := range err3.Log {
//	//			fmt.Println(l)
//	//		}
//	//	}
//	//	return nil, fmt.Errorf("error initializing kafka ebpf program: %s", err)
//	//}
//
//	//filter, _ := mgr.GetProbe(manager.ProbeIdentificationPair{EBPFSection: kafkaSocketFilterStub, EBPFFuncName: "socket__kafka_filter_entry", UID: probeUID})
//	//if filter == nil {
//	//	return nil, fmt.Errorf("error retrieving socket filter")
//	//}
//
//	//closeFilterFn, err := filterpkg.HeadlessSocketFilter(c, filter)
//	//if err != nil {
//	//	return nil, fmt.Errorf("error enabling Kafka traffic inspection: %s", err)
//	//}
//
//	telemetry, err := newTelemetry()
//	if err != nil {
//		//closeFilterFn()
//		return nil, err
//	}
//
//	statkeeper := newKafkaStatkeeper(c, telemetry)
//	//processMonitor := monitor.GetProcessMonitor()
//
//	return &Monitor{
//		ebpfProgram:   ebpfProgram,
//		telemetry:     telemetry,
//		closeFilterFn: func() {},
//		statkeeper:    statkeeper,
//		//processMonitor: processMonitor,
//	}, nil
//}
//
//// Start consuming Kafka events
//func (m *Monitor) Start() error {
//	if m == nil {
//		return nil
//	}
//
//	var err error
//	m.consumer, err = events.NewConsumer(
//		"kafka",
//		m.ebpfProgram.Manager.Manager,
//		m.process,
//	)
//	if err != nil {
//		return err
//	}
//	m.consumer.Start()
//	return nil
//
//	//if err := m.ebpfProgram.Start(); err != nil {
//	//	return err
//	//}
//
//	//return m.processMonitor.Initialize()
//}
//
//// GetKafkaStats returns a map of Kafka stats
//func (m *Monitor) GetKafkaStats() map[Key]*RequestStat {
//	if m == nil {
//		return nil
//	}
//
//	m.consumer.Sync()
//	m.telemetry.log()
//	return m.statkeeper.GetAndResetAllStats()
//}
//
//// Stop Kafka monitoring
//func (m *Monitor) Stop() {
//	if m == nil {
//		return
//	}
//
//	m.processMonitor.Stop()
//	m.ebpfProgram.Close()
//	m.consumer.Stop()
//	m.closeFilterFn()
//}
//
//func (m *Monitor) process(data []byte) {
//
//	tx := (*ebpfKafkaTx)(unsafe.Pointer(&data[0]))
//	m.telemetry.count(tx)
//	m.statkeeper.Process(tx)
//}
//
////// DumpMaps dumps the maps associated with the monitor
////func (m *Monitor) DumpMaps(maps ...string) (string, error) {
////	return m.ebpfProgram.DumpMaps(maps...)
////}

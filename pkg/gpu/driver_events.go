// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && nvml

package gpu

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxDriverEventMessageLength = 4096

	nvidiaXidFirstNVLink5Code = 144
	nvidiaXidLastNVLink5Code  = 150

	nvidiaXidMMUFaultCode         = 31
	nvidiaXidDBECode              = 48
	nvidiaXidRowRemapperCode      = 63
	nvidiaXidRetirementFailCode   = 64
	nvidiaXidSBEStormCode         = 92
	nvidiaXidContainedECCCode     = 94
	nvidiaXidUncontainedECCCode   = 95
	nvidiaXidResidualECCCode      = 140
	nvidiaXidRecoveryActionCode   = 154
	nvidiaXidTPCRetirementCode    = 156
	nvidiaXidTPCRetireFailureCode = 157
	nvidiaXidChannelRepairCode    = 160
	nvidiaXidRepairFailureCode    = 161
	nvidiaXidDRAMDetailCode       = 171
	nvidiaXidSRAMDetailCode       = 172
	nvidiaXidBankRemapCode        = 177

	memoryLocationDRAM = "DRAM"
	memoryLocationSRAM = "SRAM"

	repairTargetRow    = "row"
	repairTargetBank   = "bank"
	repairTargetTPC    = "tpc"
	repairContainerGPC = "GPC"
	stormSourceDRAM    = "dram"
	stormSourceSM      = "sm"
	spareSourceSameGPC = "same_gpc"
	spareSourceDiffGPC = "different_gpc"
)

var (
	nvidiaXidPrefixPattern       = regexp.MustCompile(`(?i)\bNVRM:\s*Xid\b`)
	nvidiaPCIBusIDPattern        = regexp.MustCompile(`(?i)\bPCI:\s*([[:xdigit:]]{4}:[[:xdigit:]]{2}:[[:xdigit:]]{2})(?:\.[0-7])?`)
	nvidiaXidCodePattern         = regexp.MustCompile(`\)\s*:\s*([0-9]+)\b`)
	nvidiaProcessIDPattern       = regexp.MustCompile(`(?i)\bpid=(\d+)`)
	nvidiaProcessNamePattern     = regexp.MustCompile(`(?i)\bname=(?:'([^']*)'|([^,\s]+))`)
	nvidiaChannelPattern         = regexp.MustCompile(`(?i)\b(?:ch|channel)\s+0x([[:xdigit:]]+)`)
	nvidiaMMUInterruptPattern    = regexp.MustCompile(`(?i)\bintr\s+([[:xdigit:]]+)`)
	nvidiaMMUFaultPattern        = regexp.MustCompile(`(?i)MMU Fault:\s*ENGINE\s+([[:alnum:]_#]+)(?:\s+([[:alnum:]_#]+))?\s+faulted\s+@\s+(?:0x)?([[:xdigit:]_]+)`)
	nvidiaMMUFaultTypePattern    = regexp.MustCompile(`\b(FAULT_[A-Z_]+)\b`)
	nvidiaMMUAccessTypePattern   = regexp.MustCompile(`\b(ACCESS_TYPE_[A-Z_]+)\b`)
	nvidiaNVLink5Pattern         = regexp.MustCompile(`(?i)\)\s*:\s*\d+\s*,\s*(?:pid=\d+,\s*name=(?:'[^']*'|[^,\s]+),\s*)?([A-Z][A-Z0-9_]*)\s+(Fatal|Nonfatal)\s+XC\s*(\d+)\s+i\s*(\d+)(?:\s+Link\s+(\d+))?`)
	nvidiaNVLinkStatusPattern    = regexp.MustCompile(`(?i)[(\[]\s*(0x[[:xdigit:]]+(?:[\s,]+0x[[:xdigit:]]+)*)\s*[)\]]`)
	nvidiaPhysicalAddressPattern = regexp.MustCompile(`(?i)\bphysAddr\s+(0x[[:xdigit:]_]+)`)
	nvidiaPartitionPattern       = regexp.MustCompile(`(?i)\bpartition\s+(\d+)\s*,\s*subpartition\s+(\d+)`)
	nvidiaRowAddressPattern      = regexp.MustCompile(`(?i)\brow(?:\s+address)?\s+(0x[[:xdigit:]_]+)`)
	nvidiaRowRemapperSitePattern = regexp.MustCompile(`(?i)\bsite\s+([[:alnum:]_:-]+)`)
	nvidiaMemoryLocationPattern  = regexp.MustCompile(`(?i)\b(SRAM|DRAM)\b`)
	// The driver prints "Marking Channel N in FBPA N" and "Marking LTS N in FPB N" — the
	// resource keyword and the container keyword both change between the two forms.
	nvidiaChannelRepairPattern    = regexp.MustCompile(`(?i)Marking\s+(Channel|LTS)\s+(\d+)\s+in\s+(FBPA|FPB)\s+(\d+)`)
	nvidiaRepairActivationPattern = regexp.MustCompile(`(?i)Perform\s+(.+?)\s+to\s+activate\s+repair`)
	nvidiaRepairFailurePattern    = regexp.MustCompile(`(?i)Repairing\s+(Channel|LTS)\s+failed\s+as\s+there\s+are\s+no\s+more\s+spare\s+([^.]+)`)
	// Row remapper and bank remapper addresses are parenthesised, not bare after the keyword.
	nvidiaRemapAddressPattern   = regexp.MustCompile(`(?i)Row Remapper(?:\s+Error)?:\s*(?:New\s+row\s*)?\((0x[[:xdigit:]]+)\)`)
	nvidiaRemapFailurePattern   = regexp.MustCompile(`(?i)Row Remapper(?:\s+Error)?:\s*\(0x[[:xdigit:]]+\)\s*-\s*([^.]+)`)
	nvidiaRetirementFailPattern = regexp.MustCompile(`(?i)DRAM Retirement failed due to\s+(.+?)\s+at\s+(0x[[:xdigit:]]+)`)
	nvidiaBankRemapPattern      = regexp.MustCompile(`(?i)Bank Remapper:\s*New\s+bank\s+marked\s+for\s+remapping`)
	nvidiaTPCRetirePattern      = regexp.MustCompile(`(?i)Retiring\s+TPC\s+(\d+)\s+from\s+GPC\s+(\d+)\s+with\s+a\s+(spare\s+from\s+the\s+same\s+GPC|TPC\s+from\s+a\s+different\s+GPC)`)
	nvidiaTPCRetireFailPattern  = regexp.MustCompile(`(?i)Unable\s+to\s+retire\s+TPC\s+(\d+)\s+from\s+GPC\s+(\d+)(\s+in\s+MIG\s+mode)?\s+as\s+there\s+are\s+no\s+spare\s+TPCs`)
	nvidiaSBEStormPattern       = regexp.MustCompile(`(?i)framebuffer\s+at\s+logical\s+partition\s+(\d+)`)
	nvidiaSMStormPattern        = regexp.MustCompile(`(?i)SM\s+SBE\s+interrupt\s+storm`)
	nvidiaResidualCountsPattern = regexp.MustCompile(`(?i)\bDRAM:(-?\d+),\s*LTC:(-?\d+),\s*MMU:(-?\d+),\s*PCIE:(-?\d+)`)
	nvidiaDRAMDetailPattern     = regexp.MustCompile(`(?i)\bFBPA\s+(\d+)\s+subpartition\s+(\d+)`)
	nvidiaRecoveryActionPattern = regexp.MustCompile(`(?i)changed\s+from\s+(0x[[:xdigit:]]+)\s+\(([^)]*)\)\s+to\s+(0x[[:xdigit:]]+)\s+\(([^)]*)\)`)
	logLimit                    = log.NewLogLimit(10, 10*time.Minute)
)

type driverEventTelemetry struct {
	once               sync.Once
	dropped            telemetry.Counter
	parseFailures      telemetry.Counter
	unresolvedPCI      telemetry.Counter
	enrichmentFailures telemetry.Counter
}

var driverEventsTelemetryDefinitions driverEventTelemetry

func (t *driverEventTelemetry) init(component telemetry.Component) {
	t.once.Do(func() {
		t.dropped = component.NewCounter("gpu__driver_events", "dropped", nil, "Number of NVIDIA driver events dropped because the queue was full")
		t.parseFailures = component.NewCounter("gpu__driver_events", "parse_failures", []string{"reason"}, "Number of NVIDIA driver events that could not be parsed")
		t.unresolvedPCI = component.NewCounter("gpu__driver_events", "unresolved_pci", nil, "Number of NVIDIA driver events whose PCI address could not be resolved to a GPU UUID")
		t.enrichmentFailures = component.NewCounter("gpu__driver_events", "enrichment_failures", nil, "Number of NVIDIA driver events with malformed optional details")
	})
}

var errDriverEventSubscriberStopped = errors.New("driver event subscriber stopped")

type driverEventReader interface {
	Errors() <-chan error
	Stop()
}

// DriverEventSubscriber reads NVIDIA Xid events from /dev/kmsg and associates them with physical GPU UUIDs.
type DriverEventSubscriber struct {
	reader      driverEventReader
	records     <-chan kernel.KmsgRecord
	unsubscribe func()
	telemetry   *driverEventTelemetry

	events      chan model.DriverEvent
	deviceCache ddnvml.DeviceCache
	stopOnce    sync.Once
	done        chan struct{}
}

// DriverEventSubscriberConfig configures a DriverEventSubscriber.
type DriverEventSubscriberConfig struct {
	QueueSize int
}

// NewDriverEventSubscriber starts a reader for future NVIDIA Xid kernel messages.
func NewDriverEventSubscriber(component telemetry.Component, deviceCache ddnvml.DeviceCache, cfg DriverEventSubscriberConfig) (*DriverEventSubscriber, error) {
	if component == nil {
		return nil, errors.New("driver event telemetry component is nil")
	}
	if deviceCache == nil {
		return nil, errors.New("GPU device cache is nil")
	}
	if cfg.QueueSize <= 0 {
		return nil, fmt.Errorf("driver event queue size must be positive, got %d", cfg.QueueSize)
	}

	reader, err := kernel.NewKmsgReader(component)
	if err != nil {
		return nil, fmt.Errorf("create kmsg reader: %w", err)
	}

	records, unsubscribe, err := reader.Subscribe("gpu-driver-events", isNvidiaXidMessage)
	if err != nil {
		reader.Stop()
		return nil, fmt.Errorf("subscribe to kmsg reader: %w", err)
	}
	driverEventsTelemetryDefinitions.init(component)
	subscriber := &DriverEventSubscriber{
		reader:      reader,
		telemetry:   &driverEventsTelemetryDefinitions,
		records:     records,
		unsubscribe: unsubscribe,
		events:      make(chan model.DriverEvent, cfg.QueueSize),
		deviceCache: deviceCache,
		done:        make(chan struct{}),
	}
	go subscriber.run()
	return subscriber, nil
}

// GetAndFlush returns queued driver events and clears the queue.
// It returns an error when the subscriber has stopped.
func (s *DriverEventSubscriber) GetAndFlush() ([]model.DriverEvent, error) {
	events := make([]model.DriverEvent, 0, len(s.events))
	for {
		select {
		case event, ok := <-s.events:
			if !ok {
				if len(events) > 0 {
					return events, nil
				}
				return events, errDriverEventSubscriberStopped
			}
			events = append(events, event)
		default:
			return events, nil
		}
	}
}

// Stop terminates the kernel reader and waits for the subscriber to exit.
func (s *DriverEventSubscriber) Stop() {
	s.stopOnce.Do(func() {
		s.reader.Stop()
		<-s.done
		if s.unsubscribe != nil {
			s.unsubscribe()
		}
	})
}

func (s *DriverEventSubscriber) run() {
	defer func() {
		close(s.events)
		close(s.done)
	}()

	for {
		select {
		case record, ok := <-s.records:
			if !ok {
				return
			}
			event, err := s.createDriverEvent(record)
			if err != nil && logLimit.ShouldLog() {
				log.Errorf("failed to create driver event: %v", err)
			} else if err == nil {
				s.enqueue(event)
			}
		case err, ok := <-s.reader.Errors():
			if ok {
				log.Errorf("GPU driver event reader stopped: %v", err)
			}
			return
		}
	}
}

func (s *DriverEventSubscriber) createDriverEvent(record kernel.KmsgRecord) (model.DriverEvent, error) {
	var event model.DriverEvent

	pciBusID, ok := parseBusID(record)
	if !ok {
		s.telemetry.parseFailures.Inc("missing_pci_bus_id")
		return event, errors.New("can't find PCI bus ID in message")
	}

	event.PCIBusID = pciBusID

	// The raw kmsg timestamp cannot be reliably converted to wall time; use the
	// timestamp captured by KmsgReader when it observed the record instead.
	event.Timestamp = record.ObservedAt

	var (
		enrichmentFailed bool
		err              error
	)
	switch {
	case isNvidiaXidMessage(record):
		enrichmentFailed, err = parseNvidiaXid(record, &event)
	default:
		err = errors.New("message not recognized")
	}

	if err != nil {
		s.telemetry.parseFailures.Inc("parse_failure")
		return event, err
	}
	if enrichmentFailed {
		s.telemetry.enrichmentFailures.Inc()
	}

	// A device that has left the PCIe bus can no longer be resolved through NVML, which is
	// exactly the state Xid 79 and the 62/119/120 sequence preceding it report. Dropping the
	// event here would discard the codes that need it most, so keep the event and let the
	// PCI bus ID carry the device identity.
	device, err := s.deviceCache.GetByPCIBusID(pciBusID)
	if err != nil {
		s.telemetry.unresolvedPCI.Inc()
		if logLimit.ShouldLog() {
			log.Warnf("emitting driver event without device UUID: resolve PCI bus ID %s: %v", pciBusID, err)
		}
		return event, nil
	}
	event.DeviceUUID = device.GetDeviceInfo().UUID

	return event, nil
}

func (s *DriverEventSubscriber) enqueue(event model.DriverEvent) {
	select {
	case s.events <- event:
	default:
		s.telemetry.dropped.Inc()
	}
}

func isNvidiaXidMessage(record kernel.KmsgRecord) bool {
	return nvidiaXidPrefixPattern.MatchString(record.Message)
}

func parseBusID(record kernel.KmsgRecord) (string, bool) {
	matches := nvidiaPCIBusIDPattern.FindStringSubmatch(record.Message)
	if matches == nil {
		return "", false
	}
	return strings.ToLower(matches[1]) + ".0", true
}

type nvidiaXidDetailParser func(message string, xid *model.NvidiaXid) bool

func parseNvidiaXid(record kernel.KmsgRecord, event *model.DriverEvent) (bool, error) {
	xidCodeMatches := nvidiaXidCodePattern.FindStringSubmatch(record.Message)
	if xidCodeMatches == nil {
		return false, errors.New("missing Xid code")
	}

	xidCode, err := strconv.ParseUint(xidCodeMatches[1], 10, 64)
	if err != nil {
		return false, fmt.Errorf("parse Xid code: %w", err)
	}

	event.Type = model.DriverEventTypeNvidiaXid
	event.NvidiaXid = &model.NvidiaXid{
		XidCode: xidCode,
		Message: truncateDriverEventMessage(record.Message),
	}
	parseNvidiaXidPreamble(record.Message, event.NvidiaXid)

	detailParser := nvidiaXidDetailParserForCode(xidCode)
	if detailParser == nil {
		return false, nil
	}
	return !detailParser(record.Message, event.NvidiaXid), nil
}

func nvidiaXidDetailParserForCode(xidCode uint64) nvidiaXidDetailParser {
	switch {
	case xidCode == nvidiaXidMMUFaultCode:
		return parseNvidiaXid31
	case xidCode >= nvidiaXidFirstNVLink5Code && xidCode <= nvidiaXidLastNVLink5Code:
		return parseNvidiaNVLink5Xid
	case xidCode == nvidiaXidDBECode ||
		xidCode == nvidiaXidSBEStormCode ||
		xidCode == nvidiaXidContainedECCCode ||
		xidCode == nvidiaXidUncontainedECCCode ||
		xidCode == nvidiaXidResidualECCCode ||
		xidCode == nvidiaXidDRAMDetailCode ||
		xidCode == nvidiaXidSRAMDetailCode:
		return func(message string, xid *model.NvidiaXid) bool {
			return parseNvidiaMemoryXid(message, xidCode, xid)
		}
	// The retirement and repair codes report which spare resource was spent. Xid 63 and 64
	// also carry a memory address, so they run both parsers.
	case xidCode == nvidiaXidRowRemapperCode ||
		xidCode == nvidiaXidRetirementFailCode ||
		xidCode == nvidiaXidTPCRetirementCode ||
		xidCode == nvidiaXidTPCRetireFailureCode ||
		xidCode == nvidiaXidChannelRepairCode ||
		xidCode == nvidiaXidRepairFailureCode ||
		xidCode == nvidiaXidBankRemapCode:
		return func(message string, xid *model.NvidiaXid) bool {
			repaired := parseNvidiaRepairXid(message, xidCode, xid)
			if xidCode == nvidiaXidRowRemapperCode || xidCode == nvidiaXidRetirementFailCode {
				return parseNvidiaMemoryXid(message, xidCode, xid) || repaired
			}
			return repaired
		}
	case xidCode == nvidiaXidRecoveryActionCode:
		return parseNvidiaRecoveryXid
	default:
		return nil
	}
}

func truncateDriverEventMessage(message string) string {
	if len(message) <= maxDriverEventMessageLength {
		return message
	}
	return message[:maxDriverEventMessageLength]
}

// parseNvidiaXidPreamble reads the fields the driver prints before any code-specific detail:
// the process, and the GPU command channel. Every code can carry them, so they are parsed
// here rather than in a per-code parser — Xid 43 for instance has no detail parser at all,
// and its channel was previously dropped.
func parseNvidiaXidPreamble(message string, xid *model.NvidiaXid) {
	if matches := nvidiaProcessIDPattern.FindStringSubmatch(message); matches != nil {
		if processID, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			xid.ProcessID = &processID
		}
	}
	if matches := nvidiaProcessNamePattern.FindStringSubmatch(message); matches != nil {
		xid.ProcessName = firstNonEmpty(matches[1], matches[2])
	}
	// The 0x prefix is required, which is what separates a command channel from Xid 160's
	// decimal "Marking Channel 3 in FBPA 2" repair target.
	if matches := nvidiaChannelPattern.FindStringSubmatch(message); matches != nil {
		xid.Channel = normalizeHex(matches[1])
	}
}

func parseNvidiaXid31(message string, xid *model.NvidiaXid) bool {
	details := &model.NvidiaXidMMUFault{}
	if matches := nvidiaMMUInterruptPattern.FindStringSubmatch(message); matches != nil {
		details.Interrupt = normalizeHex(matches[1])
	}
	if matches := nvidiaMMUFaultPattern.FindStringSubmatch(message); matches != nil {
		details.Engine = matches[1]
		details.EngineClient = matches[2]
		details.FaultAddress = normalizeHex(matches[3])
	}
	if matches := nvidiaMMUFaultTypePattern.FindStringSubmatch(message); matches != nil {
		details.FaultType = matches[1]
	}
	if matches := nvidiaMMUAccessTypePattern.FindStringSubmatch(message); matches != nil {
		details.AccessType = matches[1]
	}
	if *details != (model.NvidiaXidMMUFault{}) {
		xid.MMUFault = details
		return true
	}
	return false
}

func parseNvidiaNVLink5Xid(message string, xid *model.NvidiaXid) bool {
	matches := nvidiaNVLink5Pattern.FindStringSubmatch(message)
	if matches == nil {
		return false
	}
	details := &model.NvidiaXidNVLinkFault{
		Subcode:          matches[1],
		Fatal:            strings.EqualFold(matches[2], "Fatal"),
		CrossContainment: parseNvidiaNVLinkFlag(matches[3]),
		Injected:         parseNvidiaNVLinkFlag(matches[4]),
	}
	if matches[5] != "" {
		linkID, err := strconv.ParseUint(matches[5], 10, 64)
		if err != nil {
			return false
		}
		details.LinkID = &linkID
	}
	parseNvidiaNVLinkStatusGroup(message, details)
	xid.NVLinkFault = details
	return true
}

// parseNvidiaNVLinkFlag reads one of the line's 0/1 flag fields. The driver zero-pads these
// fields (as it does the link number), so they are compared numerically rather than textually.
func parseNvidiaNVLinkFlag(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed != 0
}

// parseNvidiaNVLinkStatusGroup reads the trailing decode group positionally. The first two
// words are intrInfo and errorStatus, the inputs NVIDIA's decode table needs; the rest is
// errorDebugData in printed order. A message-wide hex scrape cannot tell these apart.
//
// The driver parenthesises the group on R575+ and brackets it on earlier releases, so both
// forms are accepted, and the last group on the line wins.
func parseNvidiaNVLinkStatusGroup(message string, details *model.NvidiaXidNVLinkFault) {
	groups := nvidiaNVLinkStatusPattern.FindAllStringSubmatch(message, -1)
	if groups == nil {
		return
	}

	words := strings.FieldsFunc(groups[len(groups)-1][1], func(r rune) bool {
		return r == ' ' || r == '\t' || r == ','
	})
	for index, word := range words {
		switch index {
		case 0:
			details.IntrInfo = normalizeHex(word)
		case 1:
			details.ErrorStatus = normalizeHex(word)
		default:
			details.ErrorDebugData = append(details.ErrorDebugData, normalizeHex(word))
		}
	}
}

func parseNvidiaMemoryXid(message string, xidCode uint64, xid *model.NvidiaXid) bool {
	details := &model.NvidiaXidMemoryFault{}
	if matches := nvidiaPhysicalAddressPattern.FindStringSubmatch(message); matches != nil {
		details.PhysicalAddress = normalizeHex(matches[1])
	}
	if matches := nvidiaPartitionPattern.FindStringSubmatch(message); matches != nil {
		details.Partition = parseDecimalPointer(matches[1])
		details.Subpartition = parseDecimalPointer(matches[2])
	}
	// Only the bare-keyword form belongs here. The driver's parenthesised "New row (0x…)"
	// names the row being remapped rather than a fault location, so it is read by the
	// repair parser instead and is not duplicated into both structs.
	if matches := nvidiaRowAddressPattern.FindStringSubmatch(message); matches != nil {
		details.RowAddress = normalizeHex(matches[1])
	}
	if matches := nvidiaRowRemapperSitePattern.FindStringSubmatch(message); matches != nil {
		details.RowRemapperSite = matches[1]
	}
	// On Blackwell the DRAM/SRAM split arrives as a companion code rather than an inline
	// field, so for 171 and 172 the code itself is the location and no parsing is needed.
	// Only 94 and 95 carry the location in the message text.
	switch xidCode {
	case nvidiaXidDRAMDetailCode:
		details.Location = memoryLocationDRAM
	case nvidiaXidSRAMDetailCode:
		details.Location = memoryLocationSRAM
	case nvidiaXidContainedECCCode, nvidiaXidUncontainedECCCode:
		if matches := nvidiaMemoryLocationPattern.FindStringSubmatch(message); matches != nil {
			details.Location = strings.ToUpper(matches[1])
		}
	}
	if matches := nvidiaDRAMDetailPattern.FindStringSubmatch(message); matches != nil {
		details.FBPA = parseDecimalPointer(matches[1])
		details.Subpartition = parseDecimalPointer(matches[2])
	}
	if xidCode == nvidiaXidSBEStormCode {
		// Only the DRAM storm names a partition; the SM storm has no location at all.
		if matches := nvidiaSBEStormPattern.FindStringSubmatch(message); matches != nil {
			details.InterruptStormSource = stormSourceDRAM
			details.Partition = parseDecimalPointer(matches[1])
		} else if nvidiaSMStormPattern.MatchString(message) {
			details.InterruptStormSource = stormSourceSM
		}
	}
	if matches := nvidiaResidualCountsPattern.FindStringSubmatch(message); matches != nil {
		details.ResidualDRAM = parseSignedPointer(matches[1])
		details.ResidualLTC = parseSignedPointer(matches[2])
		details.ResidualMMU = parseSignedPointer(matches[3])
		details.ResidualPCIE = parseSignedPointer(matches[4])
	}
	if *details != (model.NvidiaXidMemoryFault{}) {
		xid.MemoryFault = details
		return true
	}
	return false
}

// parseNvidiaRepairXid reads the resource retirement and repair codes. Each of these lines
// names a spare resource that was spent, or a repair that could not be made, so the useful
// payload is which resource, where, and what is needed to activate the repair.
func parseNvidiaRepairXid(message string, xidCode uint64, xid *model.NvidiaXid) bool {
	details := &model.NvidiaXidRepair{}

	// "Marking Channel N in FBPA N" / "Marking LTS N in FPB N", both followed by
	// "along with its pair for repair. Perform <action> to activate repair."
	if matches := nvidiaChannelRepairPattern.FindStringSubmatch(message); matches != nil {
		details.Target = strings.ToLower(matches[1])
		details.TargetIndex = parseDecimalPointer(matches[2])
		details.Container = strings.ToUpper(matches[3])
		details.ContainerIndex = parseDecimalPointer(matches[4])
	}
	if matches := nvidiaRepairFailurePattern.FindStringSubmatch(message); matches != nil {
		details.Target = strings.ToLower(matches[1])
		details.Failed = true
		details.FailureReason = "no_spare_" + normalizeReason(matches[2])
	}
	if matches := nvidiaTPCRetirePattern.FindStringSubmatch(message); matches != nil {
		details.Target = repairTargetTPC
		details.TargetIndex = parseDecimalPointer(matches[1])
		details.Container = repairContainerGPC
		details.ContainerIndex = parseDecimalPointer(matches[2])
		if strings.Contains(strings.ToLower(matches[3]), "different") {
			details.SpareSource = spareSourceDiffGPC
		} else {
			details.SpareSource = spareSourceSameGPC
		}
	}
	if matches := nvidiaTPCRetireFailPattern.FindStringSubmatch(message); matches != nil {
		details.Target = repairTargetTPC
		details.TargetIndex = parseDecimalPointer(matches[1])
		details.Container = repairContainerGPC
		details.ContainerIndex = parseDecimalPointer(matches[2])
		details.Failed = true
		details.MIGMode = matches[3] != ""
		details.FailureReason = "no_spare_tpc"
	}
	// Row remapper: the pending form names a new row, the error form names a cause.
	if matches := nvidiaRemapAddressPattern.FindStringSubmatch(message); matches != nil {
		details.Target = repairTargetRow
		details.Address = normalizeHex(matches[1])
	}
	if matches := nvidiaRemapFailurePattern.FindStringSubmatch(message); matches != nil {
		details.Target = repairTargetRow
		details.Failed = true
		details.FailureReason = normalizeReason(matches[1])
	}
	if matches := nvidiaRetirementFailPattern.FindStringSubmatch(message); matches != nil {
		details.Failed = true
		details.FailureReason = normalizeReason(matches[1])
		details.Address = normalizeHex(matches[2])
	}
	if nvidiaBankRemapPattern.MatchString(message) {
		details.Target = repairTargetBank
	}

	if matches := nvidiaRepairActivationPattern.FindStringSubmatch(message); matches != nil {
		details.Activation = normalizeReason(matches[1])
	}
	// NVIDIA words the requirement either as the activation verb or as a bare sentence, so
	// both are checked. A node reboot means a GPU reset will not pick the repair up.
	details.NodeRebootRequired = strings.Contains(details.Activation, "node_reboot") ||
		strings.Contains(strings.ToLower(message), "node reboot")

	if *details != (model.NvidiaXidRepair{}) {
		xid.Repair = details
		return true
	}
	return false
}

// normalizeReason turns a driver phrase into a stable lower-case slug so it can be used as a
// tag value: "Row Remapping table is full" becomes "row_remapping_table_is_full".
func normalizeReason(phrase string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(phrase))), "_")
}

func parseNvidiaRecoveryXid(message string, xid *model.NvidiaXid) bool {
	matches := nvidiaRecoveryActionPattern.FindStringSubmatch(message)
	if matches == nil {
		return false
	}
	xid.RecoveryAction = &model.NvidiaXidRecoveryAction{
		PreviousCode:  parseHexPointer(matches[1]),
		PreviousLabel: matches[2],
		CurrentCode:   parseHexPointer(matches[3]),
		CurrentLabel:  matches[4],
	}
	return true
}

func normalizeHex(value string) string {
	value = strings.ReplaceAll(value, "_", "")
	if !strings.HasPrefix(strings.ToLower(value), "0x") {
		value = "0x" + value
	}
	return strings.ToLower(value)
}

func parseDecimalPointer(value string) *uint64 {
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

// parseSignedPointer is used for the Xid 140 residual counts, which the driver prints with
// %d and can report as a large negative value.
func parseSignedPointer(value string) *int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseHexPointer(value string) *uint64 {
	parsed, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(value), "0x"), 16, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

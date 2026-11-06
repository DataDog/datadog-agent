// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// EventType is a KFD SMI event ID, from enum kfd_smi_event in the kernel's
// include/uapi/linux/kfd_ioctl.h.
type EventType uint32

// KFD SMI events the GPU check counts. The kernel reports them with no
// process ID, so they reach every subscriber: no CAP_SYS_ADMIN is needed.
const (
	EventVMFault         EventType = 1 // KFD_SMI_EVENT_VMFAULT
	EventThermalThrottle EventType = 2 // KFD_SMI_EVENT_THERMAL_THROTTLE
	EventGPUPreReset     EventType = 3 // KFD_SMI_EVENT_GPU_PRE_RESET
	EventGPUPostReset    EventType = 4 // KFD_SMI_EVENT_GPU_POST_RESET
)

const (
	// DefaultKFDPath is the KFD device node, granted to containers by the AMD
	// container runtime.
	DefaultKFDPath = "/dev/kfd"

	// eventMergeWindow merges identical events of a device reported within
	// this window: one faulting kernel is reported once per XCD that hit the
	// fault, eight times on an MI300X.
	eventMergeWindow = time.Second
	// maxPendingEvents bounds the events kept between two flushes.
	maxPendingEvents = 1024
	// smiReadBufferSize holds several KFD SMI messages (96 bytes at most each).
	smiReadBufferSize = 4096
)

// amdkfdIOCSMIEvents is AMDKFD_IOC_SMI_EVENTS: _IOWR('K', 0x1F, struct
// kfd_ioctl_smi_events_args), whose argument is {u32 gpuid; u32 anon_fd}.
var amdkfdIOCSMIEvents = uintptr(3<<30 | 8<<16 | 'K'<<8 | 0x1F)

// Event is a KFD SMI event reported for a device.
type Event struct {
	DeviceUUID string
	Type       EventType
	Message    string // the event payload, after the event ID
	Time       time.Time
}

// EventSubscriber reads the KFD SMI events of AMD GPUs and keeps them until
// they are flushed. It needs read/write access to /dev/kfd, which the AMD
// container runtime grants with AMD_VISIBLE_DEVICES.
type EventSubscriber struct {
	kfdPath string
	// openSMI returns the event file of a KFD GPU ID; tests replace it.
	openSMI func(kfd *os.File, gpuID uint64) (*os.File, error)

	// readerEvent, when set by tests, is called each time a reader handles
	// an event line or stops.
	readerEvent func()

	mu       sync.Mutex
	kfd      *os.File
	subs     map[uint64]*smiSubscription // by KFD gpu_id
	pending  []Event
	dropped  int
	lastSeen map[string]time.Time // last time an event key was kept, for merging
	closed   bool
	wg       sync.WaitGroup
}

type smiSubscription struct {
	file       *os.File
	deviceUUID string
}

// NewEventSubscriber returns a subscriber reading KFD events from kfdPath.
func NewEventSubscriber(kfdPath string) *EventSubscriber {
	return &EventSubscriber{
		kfdPath:  kfdPath,
		openSMI:  openSMIEvents,
		subs:     make(map[uint64]*smiSubscription),
		lastSeen: make(map[string]time.Time),
	}
}

// Sync subscribes to the KFD GPU IDs of the devices that are not subscribed
// yet, and stops the subscriptions of the GPU IDs no longer present. It returns
// the first error opening /dev/kfd or a subscription; the other subscriptions
// are still made.
func (s *EventSubscriber) Sync(devices []*Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("event subscriber is closed")
	}

	wanted := make(map[uint64]string)
	for _, dev := range devices {
		for _, id := range dev.kfdGPUIDs {
			wanted[id] = dev.UUID
		}
	}
	for id, sub := range s.subs {
		if uuid, ok := wanted[id]; !ok || uuid != sub.deviceUUID {
			sub.file.Close() // ends its reader goroutine
			delete(s.subs, id)
		}
	}
	if len(wanted) == len(s.subs) {
		return nil
	}

	if s.kfd == nil {
		kfd, err := os.OpenFile(s.kfdPath, os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("open %s: %w", s.kfdPath, err)
		}
		s.kfd = kfd
	}
	var errs []error
	for id, uuid := range wanted {
		if _, ok := s.subs[id]; ok {
			continue
		}
		file, err := s.openSMI(s.kfd, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("subscribe to KFD events of GPU %d: %w", id, err))
			continue
		}
		sub := &smiSubscription{file: file, deviceUUID: uuid}
		s.subs[id] = sub
		s.wg.Add(1)
		go s.read(sub)
	}
	return errors.Join(errs...)
}

// Flush returns the events received since the last flush, and how many were
// dropped because too many were pending.
func (s *EventSubscriber) Flush() ([]Event, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, dropped := s.pending, s.dropped
	s.pending, s.dropped = nil, 0
	now := time.Now()
	for key, seen := range s.lastSeen {
		if now.Sub(seen) > eventMergeWindow {
			delete(s.lastSeen, key)
		}
	}
	return events, dropped
}

// Close stops every subscription and waits for their readers to return.
func (s *EventSubscriber) Close() {
	s.mu.Lock()
	s.closed = true
	for id, sub := range s.subs {
		sub.file.Close()
		delete(s.subs, id)
	}
	if s.kfd != nil {
		s.kfd.Close()
		s.kfd = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// read reads the events of one subscription until its file is closed.
func (s *EventSubscriber) read(sub *smiSubscription) {
	defer s.wg.Done()
	reader := bufio.NewReaderSize(sub.file, smiReadBufferSize)
	for {
		line, err := reader.ReadString('\n')
		if line = strings.TrimSpace(line); line != "" {
			if evt, ok := parseSMIEvent(line); ok {
				evt.DeviceUUID = sub.deviceUUID
				evt.Time = time.Now()
				s.add(evt)
			}
			s.notifyReaderEvent()
		}
		if err != nil {
			if !errors.Is(err, os.ErrClosed) {
				// The event file failed: drop the subscription so that the
				// next Sync makes a new one.
				s.drop(sub)
			}
			s.notifyReaderEvent()
			return
		}
	}
}

func (s *EventSubscriber) notifyReaderEvent() {
	if s.readerEvent != nil {
		s.readerEvent()
	}
}

// drop closes a subscription and forgets it, if it is still the current one
// for its GPU ID.
func (s *EventSubscriber) drop(sub *smiSubscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub.file.Close()
	for id, current := range s.subs {
		if current == sub {
			delete(s.subs, id)
		}
	}
}

func (s *EventSubscriber) add(evt Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := evt.DeviceUUID + "\x00" + strconv.FormatUint(uint64(evt.Type), 10) + "\x00" + evt.Message
	if last, ok := s.lastSeen[key]; ok && evt.Time.Sub(last) < eventMergeWindow {
		return
	}
	if len(s.pending) >= maxPendingEvents {
		// Not remembered, so dropped events cannot grow lastSeen.
		s.dropped++
		return
	}
	s.lastSeen[key] = evt.Time
	s.pending = append(s.pending, evt)
}

// parseSMIEvent parses a KFD SMI message: the event ID in hexadecimal, a
// space, and an event-specific payload, e.g. "1 44535:python3" for a VM fault.
func parseSMIEvent(line string) (Event, bool) {
	id, payload, _ := strings.Cut(line, " ")
	eventID, err := strconv.ParseUint(id, 16, 32)
	if err != nil {
		return Event{}, false
	}
	switch t := EventType(eventID); t {
	case EventVMFault, EventThermalThrottle, EventGPUPreReset, EventGPUPostReset:
		return Event{Type: t, Message: payload}, true
	default:
		return Event{}, false
	}
}

// openSMIEvents opens the KFD SMI event file of a GPU and enables the events
// the GPU check counts.
func openSMIEvents(kfd *os.File, gpuID uint64) (*os.File, error) {
	args := struct {
		GPUID  uint32
		AnonFD uint32
	}{GPUID: uint32(gpuID)}
	conn, err := kfd.SyscallConn()
	if err != nil {
		return nil, err
	}
	// The kernel creates the event file without O_CLOEXEC. Hold ForkLock until
	// close-on-exec is set, so that no subprocess the Agent starts inherits it
	// (and the subscription with it).
	var errno unix.Errno
	syscall.ForkLock.RLock()
	controlErr := conn.Control(func(fd uintptr) {
		_, _, errno = unix.Syscall(unix.SYS_IOCTL, fd, amdkfdIOCSMIEvents, uintptr(unsafe.Pointer(&args)))
	})
	if controlErr == nil && errno == 0 {
		unix.CloseOnExec(int(args.AnonFD))
	}
	syscall.ForkLock.RUnlock()
	if controlErr != nil {
		return nil, controlErr
	}
	if errno != 0 {
		return nil, fmt.Errorf("AMDKFD_IOC_SMI_EVENTS: %w", errno)
	}

	fd := int(args.AnonFD)
	var mask uint64
	for _, t := range []EventType{EventVMFault, EventThermalThrottle, EventGPUPreReset, EventGPUPostReset} {
		mask |= 1 << (t - 1)
	}
	buf := binary.NativeEndian.AppendUint64(nil, mask)
	if _, err := unix.Write(fd, buf); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("enable KFD events: %w", err)
	}
	// Non-blocking, so that closing the file interrupts a pending read.
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), fmt.Sprintf("kfd-smi-events-%d", gpuID)), nil
}

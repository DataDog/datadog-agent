// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	eventSetMask                  = uint64(nvml.EventTypeXidCriticalError)
	eventSetWaitTimeout           = 1000 * time.Millisecond
	devicePendingEventQueueSize   = 1000
	deviceMaxRegistrationAttempts = 5
	xidOriginDriver               = "driver"
	xidOriginHardware             = "hardware"
	xidOriginUnknown              = "unknown"
)

// helps mocking an actual events gatherer in tests
type deviceEventsCollectorCache interface {
	GetEvents(deviceUUID string) ([]ddnvml.DeviceEventData, error)
	RegisterDevice(device ddnvml.Device) error
	SupportsDevice(device ddnvml.Device) (bool, error)
}

type deviceEventsCollector struct {
	registered           bool
	registrationAttempts int
	device               ddnvml.Device
	eventsCache          deviceEventsCollectorCache
	metricsByXidCode     map[uint64]*Metric
}

func newDeviceEventsCollector(device ddnvml.Device, deps *CollectorDependencies) (c Collector, err error) {
	return newDeviceEventsCollectorWithCache(device, deps.DeviceEventsGatherer)
}

// used internally for testing
func newDeviceEventsCollectorWithCache(device ddnvml.Device, cache deviceEventsCollectorCache) (c Collector, err error) {
	if cache == nil {
		return nil, errors.New("device events gatherer cannot be nil")
	}

	if supported, err := cache.SupportsDevice(device); err != nil {
		return nil, err
	} else if !supported {
		return nil, errUnsupportedDevice
	}

	return &deviceEventsCollector{
		device:           device,
		eventsCache:      cache,
		metricsByXidCode: map[uint64]*Metric{},
	}, nil
}

func (c *deviceEventsCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *deviceEventsCollector) Name() CollectorName {
	return deviceEvents
}

func (c *deviceEventsCollector) Collect() ([]Metric, error) {
	if !c.ensureDeviceRegistered() {
		return nil, nil
	}

	events, err := c.eventsCache.GetEvents(c.DeviceUUID())
	if err != nil {
		return nil, fmt.Errorf("failed collecting device events: %w", err)
	}

	for _, evt := range events {
		if evt.EventType != nvml.EventTypeXidCriticalError {
			// currently considering only xid events
			continue
		}

		if _, ok := c.metricsByXidCode[evt.EventData]; !ok {
			xidOrigin, ok := xidCodeToOrigin[evt.EventData]
			if !ok {
				xidOrigin = xidOriginUnknown
			}
			c.metricsByXidCode[evt.EventData] = &Metric{
				Name:     "errors.xid.total",
				Type:     metrics.GaugeType,
				Priority: Medium,
				Tags: []string{
					"type:" + strconv.Itoa(int(evt.EventData)),
					"origin:" + xidOrigin,
				},
			}
		}

		c.metricsByXidCode[evt.EventData].Value++
	}

	var metrics []Metric
	for _, m := range c.metricsByXidCode {
		metrics = append(metrics, *m)
	}
	return metrics, nil
}

// note: watching device events seems to require specific permission/status with the NVIDIA driver,
// which leads to data races at initialization/node-setup time that give us error when registering
// devices. As such, this collector performs a lazy registration of the device to the events gatherer,
// with a retry logic with a max number of attempts. This makes sure we attempt registration multiple
// times at subsequent runs of the GPU check, and eventually give up if errors are persistent
func (c *deviceEventsCollector) ensureDeviceRegistered() bool {
	if c.registered {
		return true
	}

	if c.registrationAttempts >= deviceMaxRegistrationAttempts {
		return false
	}

	c.registrationAttempts++
	if err := c.eventsCache.RegisterDevice(c.device); err != nil {
		if c.registrationAttempts == 1 {
			log.Warnf("could not register %s to device events gatherer, will retry up to %d times: %v", c.DeviceUUID(), deviceMaxRegistrationAttempts, err)
		} else if c.registrationAttempts >= deviceMaxRegistrationAttempts {
			log.Warnf("could not register %s to device events gatherer after %d attempts, skipping collection: %v", c.DeviceUUID(), deviceMaxRegistrationAttempts, err)
		}
		return false
	}

	c.registered = true
	return true
}

// NewDeviceEventsGatherer creates a new cache that gathers NVML device events
func NewDeviceEventsGatherer() *DeviceEventsGatherer {
	return &DeviceEventsGatherer{
		devices: map[string]*deviceEventsEventsCache{},
	}
}

type deviceEventsEventsCache struct {
	latestEvents  []ddnvml.DeviceEventData
	pendingEvents chan ddnvml.DeviceEventData
}

// DeviceEventsGatherer asynchronously collects nvidia device events through the nvmlEventSetWait api
type DeviceEventsGatherer struct {
	running    atomic.Bool
	wg         sync.WaitGroup
	lib        ddnvml.SafeNVML
	evtSetMtx  sync.Mutex
	evtSet     nvml.EventSet
	devicesMtx sync.Mutex
	devices    map[string]*deviceEventsEventsCache // uuid -> cache
}

// Started returns true if event collection has been started
func (c *DeviceEventsGatherer) Started() bool {
	return c.running.Load()
}

// Start initializes the gatherer and starts event collection
func (c *DeviceEventsGatherer) Start() error {
	if c.running.Load() {
		return nil
	}

	c.evtSetMtx.Lock()
	defer c.evtSetMtx.Unlock()

	lib, err := ddnvml.GetSafeNvmlLib()
	if err != nil {
		return fmt.Errorf("failed to get NVML library: %w", err)
	}
	c.lib = lib

	c.evtSet, err = c.lib.EventSetCreate()
	if err != nil {
		return fmt.Errorf("failed to create NVML event set: %w", err)
	}

	c.running.Store(true)
	c.wg.Add(1)
	go c.asyncFetchWorker()
	return nil
}

// Stop deinitializes the gatherer and stops event collection
func (c *DeviceEventsGatherer) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.running.Store(false)
	c.wg.Wait()

	c.evtSetMtx.Lock()
	defer c.evtSetMtx.Unlock()
	if err := c.lib.EventSetFree(c.evtSet); err != nil {
		log.Errorf("failed freeing event set: %v", err)
	}
	c.evtSet = nil

	c.devicesMtx.Lock()
	defer c.devicesMtx.Unlock()
	for _, cache := range c.devices {
		close(cache.pendingEvents)
	}
	clear(c.devices)

	return nil
}

func (c *DeviceEventsGatherer) getDeviceCache(deviceUUID string) *deviceEventsEventsCache {
	c.devicesMtx.Lock()
	defer c.devicesMtx.Unlock()

	if cache, ok := c.devices[deviceUUID]; ok {
		return cache
	}
	return nil
}

// GetRegisteredDeviceUUIDs returns the list of device UUIDs registered for event collection
func (c *DeviceEventsGatherer) GetRegisteredDeviceUUIDs() []string {
	c.devicesMtx.Lock()
	defer c.devicesMtx.Unlock()

	uuids := slices.Collect(maps.Keys(c.devices))
	slices.Sort(uuids) // make order deterministic
	return uuids
}

// Refresh consumes the pending events (gathered in async) and populates the cache
// of latest events for each device, retrievable through GetEvents. In case
// there is no event pending since the last invocation of Refresh, GetEvents will
// return an empty event slice.
func (c *DeviceEventsGatherer) Refresh() error {
	for _, uuid := range c.GetRegisteredDeviceUUIDs() {
		cache := c.getDeviceCache(uuid)
		if cache == nil {
			log.Debugf("event set gatherer: could not find cache for %s while refreshing", uuid)
			continue
		}
		cache.latestEvents = nil
		nPending := len(cache.pendingEvents)
		for range nPending {
			cache.latestEvents = append(cache.latestEvents, <-cache.pendingEvents)
		}
	}
	return nil
}

// GetEvents returns the latest batch of cached events for the given device UUID.
// Calls to GetEvents are idempotent up until the next invocation of Refresh.
func (c *DeviceEventsGatherer) GetEvents(deviceUUID string) ([]ddnvml.DeviceEventData, error) {
	if cache := c.getDeviceCache(deviceUUID); cache != nil {
		return c.getDeviceCache(deviceUUID).latestEvents, nil
	}
	log.Debugf("event set gatherer: could not find cache for %s while getting events", deviceUUID)
	return nil, nil
}

// SupportsDevice returns true if the gatherer supports the given device
func (c *DeviceEventsGatherer) SupportsDevice(device ddnvml.Device) (bool, error) {
	evtTypes, err := device.GetSupportedEventTypes()
	if err != nil {
		if ddnvml.IsAPIUnsupportedOnDevice(err, device) {
			return false, nil
		}

		return false, fmt.Errorf("failed to query supported device event types for %s: %w", device.GetDeviceInfo().UUID, err)
	}
	return (evtTypes & eventSetMask) != 0, nil
}

// RegisterDevice registers a device for event collection
func (c *DeviceEventsGatherer) RegisterDevice(device ddnvml.Device) error {
	evtTypes, err := device.GetSupportedEventTypes()
	if err != nil {
		return fmt.Errorf("failed to query supported device event types: %w", err)
	}

	if (evtTypes & eventSetMask) == 0 {
		return errUnsupportedDevice
	}

	// device registrations might happen after the async gatherer is already started.
	// NVIDIA docs do not mention much about thread safety, so we opt for protecting
	// the event set manually for safety
	c.evtSetMtx.Lock()
	defer c.evtSetMtx.Unlock()
	if c.evtSet == nil {
		return errors.New("failed registering device events on stopped gatherer")
	}
	if err := device.RegisterEvents(evtTypes&eventSetMask, c.evtSet); err != nil {
		return fmt.Errorf("failed registering device events: %w", err)
	}

	// mark device as registered and create its cache
	c.devicesMtx.Lock()
	defer c.devicesMtx.Unlock()
	c.devices[device.GetDeviceInfo().UUID] = &deviceEventsEventsCache{
		pendingEvents: make(chan ddnvml.DeviceEventData, devicePendingEventQueueSize),
	}

	return nil
}

func (c *DeviceEventsGatherer) asyncFetchWorker() {
	defer c.wg.Done()

	log.Debugf("event set gatherer: starting async worker")
	defer log.Debugf("event set gatherer: stopping async worker")

	for {
		if !c.running.Load() {
			return
		}

		c.evtSetMtx.Lock()
		evt, err := c.lib.EventSetWait(c.evtSet, eventSetWaitTimeout)
		c.evtSetMtx.Unlock()
		if ddnvml.IsTimeout(err) {
			continue
		}
		if err != nil {
			log.Debugf("event set gatherer: error during wait: %s", err.Error())
			continue
		}
		if evt.DeviceUUID == "" {
			continue
		}

		devCache := c.getDeviceCache(evt.DeviceUUID)
		if devCache == nil {
			log.Debugf("event set gatherer: could not find cache for %s while fetching", evt.DeviceUUID)
			continue
		}

		select {
		case devCache.pendingEvents <- evt:
		default:
			log.Debugf("event set gatherer: event discarded for device %s", evt.DeviceUUID)
		}
	}
}

// see https://docs.nvidia.com/deploy/xid-errors/index.html#working-with-xid-errors
// this is an opinionated categorization of xid failure codes based on the documented
// possible causes. The goal is to provide the metric with a readable tag of where the
// issue can come from (to at least spot device/driver errors). Unused xid codes are omitted.
var xidCodeToOrigin = map[uint64]string{
	1:   xidOriginHardware, // Invalid or corrupted push buffer stream
	2:   xidOriginHardware, // Invalid or corrupted push buffer stream
	3:   xidOriginHardware, // Invalid or corrupted push buffer stream
	4:   xidOriginHardware, // GPU semaphore timeout
	6:   xidOriginHardware, // Invalid or corrupted push buffer stream
	7:   xidOriginHardware, // Invalid or corrupted push buffer address
	8:   xidOriginHardware, // GPU stopped processing
	9:   xidOriginDriver,   // Driver error programming GPU
	11:  xidOriginHardware, // Invalid or corrupted push buffer stream
	12:  xidOriginDriver,   // Driver error handling GPU exception
	13:  xidOriginHardware, // Graphics Engine Exception
	16:  xidOriginDriver,   // Display engine hung
	18:  xidOriginDriver,   // Bus mastering disabled in PCI Config Space
	19:  xidOriginDriver,   // Display Engine error
	20:  xidOriginHardware, // Invalid or corrupted Mpeg push buffer
	21:  xidOriginHardware, // Invalid or corrupted Motion Estimation push buffer
	22:  xidOriginHardware, // Invalid or corrupted Video Processor push buffer
	24:  xidOriginHardware, // GPU semaphore timeout
	25:  xidOriginHardware, // Invalid or illegal push buffer stream
	26:  xidOriginDriver,   // Framebuffer timeout
	27:  xidOriginDriver,   // Video processor exception
	28:  xidOriginDriver,   // Video processor exception
	29:  xidOriginDriver,   // Video processor exception
	30:  xidOriginDriver,   // GPU semaphore access error
	31:  xidOriginHardware, // GPU memory page fault
	32:  xidOriginHardware, // Invalid or corrupted push buffer stream
	33:  xidOriginDriver,   // Internal micro-controller error
	34:  xidOriginDriver,   // Video processor exception
	35:  xidOriginDriver,   // Video processor exception
	36:  xidOriginDriver,   // Video processor exception
	37:  xidOriginDriver,   // Driver firmware error
	38:  xidOriginDriver,   // Driver firmware error
	42:  xidOriginDriver,   // Video processor exception
	43:  xidOriginDriver,   // GPU stopped processing
	44:  xidOriginDriver,   // Graphics Engine fault during context switch
	45:  xidOriginDriver,   // Preemptive cleanup due to previous errors
	46:  xidOriginDriver,   // GPU stopped processing
	47:  xidOriginDriver,   // Video processor exception
	48:  xidOriginHardware, // Double Bit ECC Error
	54:  xidOriginHardware, // Auxiliary power is not connected to the GPU board
	56:  xidOriginDriver,   // Display Engine error
	57:  xidOriginHardware, // Error programming video memory interface
	58:  xidOriginHardware, // Unstable video memory interface detected
	59:  xidOriginDriver,   // Internal micro-controller error (older drivers)
	60:  xidOriginDriver,   // Video processor exception
	61:  xidOriginDriver,   // Internal micro-controller breakpoint/warning
	62:  xidOriginDriver,   // Internal micro-controller halt (newer drivers)
	63:  xidOriginHardware, // ECC page retirement or row remapping recording event
	64:  xidOriginHardware, // ECC page retirement or row remapper recording failure
	65:  xidOriginDriver,   // Video processor exception
	66:  xidOriginDriver,   // Illegal access by driver
	67:  xidOriginDriver,   // Illegal access by driver
	68:  xidOriginHardware, // NVDEC0 Exception
	69:  xidOriginHardware, // Graphics Engine class error
	70:  xidOriginHardware, // Unknown Error
	71:  xidOriginHardware, // Unknown Error
	72:  xidOriginHardware, // Unknown Error
	73:  xidOriginHardware, // NVENC2 Error
	74:  xidOriginHardware, // NVLINK Error
	75:  xidOriginHardware, // Unknown Error
	76:  xidOriginHardware, // Unknown Error
	77:  xidOriginHardware, // Unknown Error
	78:  xidOriginDriver,   // vGPU Start Error
	79:  xidOriginHardware, // GPU has fallen off the bus
	80:  xidOriginHardware, // Corrupted data sent to GPU
	81:  xidOriginHardware, // VGA Subsystem Error
	82:  xidOriginHardware, // NVJPG0 Error
	83:  xidOriginHardware, // NVDEC1 Error
	84:  xidOriginHardware, // NVDEC2 Error
	85:  xidOriginHardware, // Unknown Error
	86:  xidOriginHardware, // OFA Exception
	88:  xidOriginHardware, // NVDEC3 Error
	89:  xidOriginHardware, // NVDEC4 Error
	92:  xidOriginHardware, // High single-bit ECC error rate
	93:  xidOriginDriver,   // Non-fatal violation of provisioned InfoROM wear limit
	94:  xidOriginHardware, // Contained ECC error
	95:  xidOriginHardware, // Uncontained ECC error
	96:  xidOriginHardware, // NVDEC5 Error
	97:  xidOriginHardware, // NVDEC6 Error
	98:  xidOriginHardware, // NVDEC7 Error
	99:  xidOriginHardware, // NVJPG1 Error
	100: xidOriginHardware, // NVJPG2 Error
	101: xidOriginHardware, // NVJPG3 Error
	102: xidOriginHardware, // NVJPG4 Error
	103: xidOriginHardware, // NVJPG5 Error
	104: xidOriginHardware, // NVJPG6 Error
	105: xidOriginHardware, // NVJPG7 Error
	106: xidOriginHardware, // SMBPBI Test Message
	107: xidOriginHardware, // SMBPBI Test Message Silent
	109: xidOriginHardware, // Context Switch Timeout Error
	110: xidOriginHardware, // Security Fault Error
	111: xidOriginHardware, // Display Bundle Error Event
	112: xidOriginHardware, // Display Supervisor Error
	113: xidOriginHardware, // DP Link Training Error
	114: xidOriginHardware, // Display Pipeline Underflow Error
	115: xidOriginHardware, // Display Core Channel Error
	116: xidOriginHardware, // Display Window Channel Error
	117: xidOriginHardware, // Display Cursor Channel Error
	118: xidOriginHardware, // Display Pixel Pipeline Error
	119: xidOriginHardware, // GSP RPC Timeout
	120: xidOriginHardware, // GSP Error
	121: xidOriginHardware, // C2C Link Error
	122: xidOriginHardware, // SPI PMU RPC Read Failure
	123: xidOriginHardware, // SPI PMU RPC Write Failure
	124: xidOriginHardware, // SPI PMU RPC Erase Failure
	125: xidOriginHardware, // Inforom FS Failure
	137: xidOriginHardware, // NVLink FLA privilege error
	140: xidOriginHardware, // Unrecovered ECC Error
	143: xidOriginHardware, // GPU Initialization Failure
}

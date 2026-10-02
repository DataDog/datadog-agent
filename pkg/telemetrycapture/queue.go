// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"slices"
	"sync"
	"time"
	"unsafe"
)

// RecordOverhead conservatively covers envelope strings, queue bookkeeping,
// and the reservation. Payload storage must be reserved separately.
const RecordOverhead int64 = 2048

type entry struct {
	record   *Record
	bytes    int64
	pins     int
	retired  bool
	released bool
}

// Reservation accounts for one logical item, including unfinished assembly.
// An adapter must call Grow BEFORE allocating each additional owned copy, and
// eventually Commit or Discard. A reservation must not escape its observation
// worker. Deferred Discard ensures early returns do not strand a reservation.
type Reservation struct {
	manager *Manager
	session *session
	entry   *entry
	record  Record
	done    bool
}

// Begin admits a logical observation without waiting for queue capacity. The
// disabled path is just an atomic load. Capacity exhaustion fails capture only.
func (m *Manager) Begin(stream Stream, collectedAt time.Time, cadence time.Duration, bytes int64) *Reservation {
	return m.begin(Control{}, stream, collectedAt, cadence, bytes)
}

// BeginFor reserves only in the session selected before an adapter inspected
// borrowed producer data. A late callback cannot contribute to its successor.
func (m *Manager) BeginFor(control Control, stream Stream, collectedAt time.Time, cadence time.Duration, bytes int64) *Reservation {
	if validateControl(control) != nil {
		return nil
	}
	return m.begin(control, stream, collectedAt, cadence, bytes)
}

func (m *Manager) begin(control Control, stream Stream, collectedAt time.Time, cadence time.Duration, bytes int64) *Reservation {
	if m == nil {
		return nil
	}
	s := m.active.Load()
	if s == nil || (control.SessionID != "" && s.id != control.SessionID) {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	if m.active.Load() != s || !slices.Contains(s.streams, stream) {
		return nil
	}
	if bytes < 0 || bytes > MaxItemBytes-RecordOverhead || s.records >= MaxRecords || s.bytes > MaxBytes-RecordOverhead-bytes {
		m.failLocked(s, true)
		return nil
	}
	if collectedAt.IsZero() || cadence <= 0 {
		m.failLocked(s, false)
		return nil
	}
	e := &entry{bytes: RecordOverhead + bytes}
	s.bytes += e.bytes
	s.records++
	s.pending++
	m.cycle++
	return &Reservation{
		manager: m, session: s, entry: e,
		record: Record{
			ProtocolVersion: ProtocolVersion, SessionID: s.id, Producer: m.identity,
			CycleID: m.cycle, Stream: stream, CollectedAt: collectedAt,
			ObservedAt: time.Now(), Cadence: cadence,
		},
	}
}

// Control identifies the reservation's session, even after that session stops.
func (r *Reservation) Control() Control {
	return Control{ProtocolVersion: ProtocolVersion, SessionID: r.session.id}
}

// Grow reserves additional memory for an admitted logical item. Stop waits for
// this already admitted item; it prevents Begin from admitting another one.
func (r *Reservation) Grow(bytes int64) bool {
	if r == nil {
		return false
	}
	m, s := r.manager, r.session
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	if r.done || s.state == Failed || s.state == Stopped {
		return false
	}
	if bytes < 0 || bytes > MaxItemBytes-r.entry.bytes || bytes > MaxBytes-s.bytes {
		m.failLocked(s, true)
		return false
	}
	r.entry.bytes += bytes
	s.bytes += bytes
	return true
}

// Commit transfers ownership of a complete payload to capture. The adapter must
// not mutate or retain its owned buffers afterward. Size checking catches an
// incorrectly sized projection; it is not a substitute for reserving BEFORE
// copying. No serialization or I/O happens here.
func (r *Reservation) Commit(payload Payload) bool {
	if r == nil {
		return false
	}
	size := PayloadSize(payload)
	m, s := r.manager, r.session
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	if r.done {
		return false
	}
	if size > r.entry.bytes-RecordOverhead || !validPayload(r.record.Stream, payload) {
		m.failLocked(s, false)
	}
	r.done = true
	s.pending--
	if s.state == Failed {
		r.entry.retired = true
		m.releaseLocked(s, r.entry)
		m.notifyLocked(s)
		return false
	}
	s.sequence++
	record := r.record
	record.Sequence = s.sequence
	record.Payload = payload
	r.entry.record = &record
	s.queue = append(s.queue, r.entry)
	m.notifyLocked(s)
	return true
}

// Discard finishes an observation with no selected output. It is also safe in
// a defer after Commit. Adapters must Fail if discarded output was incomplete.
func (r *Reservation) Discard() {
	if r == nil {
		return
	}
	m, s := r.manager, r.session
	m.mu.Lock()
	defer m.mu.Unlock()
	if r.done {
		return
	}
	r.done = true
	s.pending--
	r.entry.retired = true
	m.releaseLocked(s, r.entry)
	m.notifyLocked(s)
	m.finishLocked(s)
}

// Observe is the single-copy adapter helper. copyPayload runs only after the
// reservation succeeds. A panic in capture copying fails capture, never normal
// submission. Its return value is diagnostic and must not be a submission error.
func (m *Manager) Observe(stream Stream, collectedAt time.Time, cadence time.Duration, bytes int64, copyPayload func() Payload) (accepted bool) {
	r := m.Begin(stream, collectedAt, cadence, bytes)
	if r == nil {
		return false
	}
	defer r.Discard()
	defer func() {
		if recover() != nil {
			_ = m.Fail(r.Control())
			accepted = false
		}
	}()
	return r.Commit(copyPayload())
}

// Batch pins immutable records while a reader uses them. Release MUST be called,
// including on cancellation. Pins remain budgeted after acknowledgement/failure.
// At most one batch can be outstanding, bounding reader-owned memory as well.
type Batch struct {
	Records       []*Record
	Status        Status
	manager       *Manager
	session       *session
	entries       []*entry
	encodingBytes int64
	once          sync.Once
}

// Read acknowledges the previously delivered prefix and returns a bounded batch.
// Repeating a cursor only re-reads remaining records; it creates no observations.
func (m *Manager) Read(request ReadRequest) (*Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sessionLocked(request.Control)
	if err != nil {
		return nil, err
	}
	if s.state == Failed {
		return nil, ErrFailed
	}
	if request.Cursor < s.ack || request.Cursor > s.delivered {
		return nil, ErrCursor
	}
	// The single outstanding reader is tracked separately from record pins, since
	// an empty batch also owns bookkeeping and potentially a blocked IPC writer.
	if s.reading {
		return nil, ErrBusy
	}
	s.ack = request.Cursor
	n := 0
	for n < len(s.queue) && s.queue[n].record.Sequence <= s.ack {
		e := s.queue[n]
		e.retired = true
		m.releaseLocked(s, e)
		n++
	}
	s.queue = slices.Delete(s.queue, 0, n)
	m.finishLocked(s)
	batch := &Batch{manager: m, session: s}
	for _, e := range s.queue {
		if len(batch.Records) == MaxBatchRecords {
			break
		}
		e.pins++
		batch.entries = append(batch.entries, e)
		batch.Records = append(batch.Records, e.record)
		s.delivered = e.record.Sequence
	}
	s.reading = true
	batch.Status = m.statusLocked()
	return batch, nil
}

// reserveEncoding accounts for the bounded IPC writer, including its JSON and
// base64 scratch space. It never runs on a production submission path.
func (b *Batch) reserveEncoding() bool {
	m, s := b.manager, b.session
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.state == Failed {
		return false
	}
	const bytes int64 = 16 << 10
	if bytes > MaxBytes-s.bytes {
		m.failLocked(s, true)
		return false
	}
	s.bytes += bytes
	b.encodingBytes = bytes
	return true
}

// Release ends this read's ownership of records and encoding scratch space.
func (b *Batch) Release() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		m, s := b.manager, b.session
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, e := range b.entries {
			e.pins--
			m.releaseLocked(s, e)
		}
		s.bytes -= b.encodingBytes
		s.reading = false
		b.Records, b.entries = nil, nil
		m.finishLocked(s)
	})
}

func (m *Manager) releaseLocked(s *session, e *entry) {
	if e.retired && e.pins == 0 && !e.released {
		e.record = nil
		e.released = true
		s.bytes -= e.bytes
		s.records--
	}
}

func validPayload(stream Stream, p Payload) bool {
	kinds := 0
	if len(p.Series) != 0 {
		kinds++
	}
	if p.Metadata != nil {
		kinds++
	}
	if p.Software != nil {
		kinds++
	}
	if p.Inventory != nil {
		kinds++
	}
	if len(p.Chunks) != 0 {
		kinds++
	}
	if kinds != 1 {
		return false
	}
	switch stream {
	case Metrics:
		return len(p.Series) != 0
	case Metadata:
		return p.Metadata != nil
	case Software:
		return p.Software != nil
	case AgentInventory:
		return p.Inventory != nil && p.Inventory.Agent != nil && p.Inventory.Host == nil && p.Inventory.SystemInfo == nil
	case HostInventory:
		return p.Inventory != nil && p.Inventory.Host != nil && p.Inventory.Agent == nil && p.Inventory.SystemInfo == nil
	case HostSystemInfo:
		return p.Inventory != nil && p.Inventory.SystemInfo != nil && p.Inventory.Agent == nil && p.Inventory.Host == nil
	case Processes, Connections:
		return len(p.Chunks) != 0
	}
	return false
}

// PayloadSize is a conservative charge for an owned projection. Adapters may
// use it on borrowed input to reserve BEFORE cloning. Slice capacities (not
// lengths) and map overhead are included; adapters must clone strings rather
// than retain substrings backed by larger production buffers.
func PayloadSize(p Payload) int64 {
	n := int64(unsafe.Sizeof(p))
	n += InventorySize(p.Inventory)
	n += int64(cap(p.Series)) * int64(unsafe.Sizeof(Series{}))
	for _, s := range p.Series {
		n += int64(len(s.Name)+len(s.Host)+len(s.Device)+len(s.Unit)+len(s.SourceTypeName)) + stringSliceSize(s.Tags)
		n += int64(cap(s.Points)) * int64(unsafe.Sizeof(Point{}))
		n += int64(cap(s.Resources)) * int64(unsafe.Sizeof(Resource{}))
		for _, resource := range s.Resources {
			n += int64(len(resource.Type) + len(resource.Name))
		}
	}
	if h := p.Metadata; h != nil {
		n += int64(unsafe.Sizeof(*h))
		n += int64(len(h.AgentVersion) + len(h.UUID) + len(h.Hostname) + len(h.OS) + len(h.AgentFlavor) + len(h.Machine) + len(h.Platform) + len(h.MacVersion) + len(h.MacMachine) + len(h.NetworkID) + len(h.PythonVersion) + len(h.PythonRuntimeVersion) + len(h.Processor) + len(h.PublicIPv4))
		n += int64(len(h.Gohai))
		n += stringSliceSize(h.Windows) + stringSliceSize(h.MacReleaseInfo) + stringSliceSize(h.UnixVersion) + stringSliceSize(h.FreeBSDVersion) + 256
		if meta := h.Meta; meta != nil {
			n += int64(unsafe.Sizeof(*meta)) + stringSliceSize(meta.Timezones) + stringSliceSize(meta.HostAliases)
			n += int64(len(meta.SocketHostname) + len(meta.SocketFqdn) + len(meta.EC2Hostname) + len(meta.Hostname) + len(meta.InstanceID) + len(meta.AgentHostname) + len(meta.ClusterName) + len(meta.LegacyResolutionHostname) + len(meta.CanonicalCloudResourceID))
		}
		if install := h.InstallMethod; install != nil {
			n += int64(unsafe.Sizeof(*install)) + int64(len(install.ToolVersion))
			for _, value := range []*string{install.Tool, install.InstallerVersion} {
				if value != nil {
					n += int64(unsafe.Sizeof(*value)) + int64(len(*value))
				}
			}
		}
		n += 256
		for key, value := range h.ContainerMeta {
			n += 256 + int64(len(key)+len(value))
		}
		if h.Proxy != nil {
			n += int64(unsafe.Sizeof(*h.Proxy))
		}
		if logs := h.Logs; logs != nil {
			n += int64(unsafe.Sizeof(*logs)) + int64(len(logs.Transport))
		}
		for key, tags := range h.HostTags {
			n += 256 + int64(len(key)) + stringSliceSize(tags)
		}
	}
	if s := p.Software; s != nil {
		n += int64(unsafe.Sizeof(*s)) + int64(cap(s.Body))
	}
	n += int64(cap(p.Chunks)) * int64(unsafe.Sizeof(Chunk{}))
	for _, c := range p.Chunks {
		n += int64(cap(c.Body)) + 256
		for k, v := range c.Headers {
			n += 256 + int64(len(k)+len(v))
		}
	}
	return n
}

func stringSliceSize(values []string) int64 {
	n := int64(cap(values)) * int64(unsafe.Sizeof(""))
	for _, v := range values {
		n += int64(len(v))
	}
	return n
}

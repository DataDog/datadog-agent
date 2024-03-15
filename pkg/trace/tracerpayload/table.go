package tracerpayload

import (
	"math"

	"github.com/vmihailenco/msgpack/v4"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

type TablePayload struct {
	chunks          []*TableTraceChunk
	containerID     string
	env             string
	hostname        string
	appVersion      string
	languageName    string
	languageVersion string
	runtimeID       string
	tracerVersion   string
	tags            map[string]string
}

func (t *TablePayload) SetTracerVersion(tv string) {
	t.tracerVersion = tv
}

func (t *TablePayload) SetContainerID(cid string) {
	t.containerID = cid
}

func (t *TablePayload) SetLanguageVersion(lv string) {
	t.languageVersion = lv
}

func (t *TablePayload) SetLanguageName(ln string) {
	t.languageName = ln
}

func UnmarshallTablePayload(b []byte) (Generic, error) {
	var payload MsgpPayload
	err := msgpack.Unmarshal(b, &payload)
	if err != nil {
		return nil, err
	}

	ttcs := MakeTraces(&payload)
	return &TablePayload{
		chunks:          ttcs,
		containerID:     "",
		env:             "",
		hostname:        "",
		appVersion:      "",
		languageName:    "",
		languageVersion: "",
		runtimeID:       "",
		tracerVersion:   "",
		tags:            nil,
	}, nil
}

func (t *TablePayload) NumChunks() int {
	return len(t.chunks)
}

func (t *TablePayload) Chunk(i int) TraceChunk {
	return t.chunks[i]
}

func (t *TablePayload) CloneChunks() []TraceChunk {
	clone := make([]TraceChunk, len(t.chunks))
	for i := 0; i < len(t.chunks); i++ {
		clone[i] = t.chunks[i]
	}
	return clone
}

func (t *TablePayload) ReplaceChunk(i int, chunk TraceChunk) {
	nc := chunk.(*TableTraceChunk) //TODO: uhhh this won't always work OR MAYBE IT WILL!?
	t.chunks[i] = nc
}

func (t *TablePayload) SetChunks(chunks []TraceChunk) {
	cs := make([]*TableTraceChunk, len(chunks))
	for i := 0; i < len(chunks); i++ {
		ttc := chunks[i].(*TableTraceChunk) //TODO: uhhh this won't always work
		cs[i] = ttc
	}
	t.chunks = cs
}

func (t *TablePayload) RemoveChunk(i int) {
	if i < 0 || i >= len(t.chunks) {
		return
	}
	t.chunks[i] = t.chunks[len(t.chunks)-1]
	t.chunks = t.chunks[:len(t.chunks)-1]
}

func (t *TablePayload) ContainerID() string {
	return t.containerID
}

func (t *TablePayload) SetTag(k string, v string) {
	if t.tags == nil {
		t.tags = map[string]string{}
	}
	t.tags[k] = v
}

func (t *TablePayload) Env() string {
	return t.env
}

func (t *TablePayload) SetEnv(e string) {
	t.env = e
}

func (t *TablePayload) Hostname() string {
	return t.hostname
}

func (t *TablePayload) SetHostname(h string) {
	t.hostname = h
}

func (t *TablePayload) AppVersion() string {
	return t.appVersion
}

func (t *TablePayload) SetAppVersion(av string) {
	t.appVersion = av
}

func (t *TablePayload) Cut(i int) Generic {
	if i < 0 {
		i = 0
	}
	if i > len(t.chunks) {
		i = len(t.chunks)
	}
	//nolint:revive // TODO(APM) Fix revive linter
	new := TablePayload{
		containerID:     t.containerID,
		languageName:    t.languageName,
		languageVersion: t.languageVersion,
		tracerVersion:   t.tracerVersion,
		runtimeID:       t.runtimeID,
		env:             t.env,
		hostname:        t.hostname,
		appVersion:      t.appVersion,
		tags:            t.tags,
	}

	new.chunks = t.chunks[:i]
	t.chunks = t.chunks[i:]

	return &new
}

func (t *TablePayload) ToPb() *pb.TracerPayload {
	chunks := make([]*pb.TraceChunk, len(t.chunks))
	for i, ttc := range t.chunks {
		chunks[i] = ttc.ToPb()
	}
	p := pb.TracerPayload{
		ContainerID:     t.containerID,
		LanguageName:    t.languageName,
		LanguageVersion: t.languageVersion,
		TracerVersion:   t.tracerVersion,
		RuntimeID:       t.runtimeID,
		Chunks:          chunks,
		Tags:            t.tags,
		Env:             t.env,
		Hostname:        t.hostname,
		AppVersion:      t.appVersion,
	}
	return &p
}

func (t *TablePayload) Serialize() ([]byte, error) {
	panic("not impld")
}

func (t *TablePayload) IsCompressed() bool {
	return true
}

type TableTraceChunk struct {
	StringTable  *StringTable
	Spans        []*TableSpan
	priority     int32
	origin       string
	droppedTrace bool
}

func (t *TableTraceChunk) NumSpans() int {
	return len(t.Spans)
}

func (t *TableTraceChunk) Span(i int) Span {
	return t.Spans[i]
}

func (t *TableTraceChunk) Priority() int32 {
	return t.priority
}

func (t *TableTraceChunk) SetPriority(p int32) {
	t.priority = p
}

func (t *TableTraceChunk) Origin() string {
	return t.origin
}

func (t *TableTraceChunk) SetOrigin(o string) {
	t.origin = o
}

func (t *TableTraceChunk) DroppedTrace() bool {
	return t.droppedTrace
}

func (t *TableTraceChunk) SetDroppedTrace(b bool) {
	t.droppedTrace = b
}

func (t *TableTraceChunk) Msgsize() int {
	//TODO actually implement me
	// for now we just are guessing
	return 20 * len(t.StringTable.strings) * len(t.Spans)
}

func (t *TableTraceChunk) ToPb() *pb.TraceChunk {
	spans := make([]*pb.Span, len(t.Spans))
	for i, ts := range t.Spans {
		spans[i] = ts.ToPb()
	}
	return &pb.TraceChunk{
		Priority:     t.priority,
		Origin:       t.origin,
		Spans:        spans,
		Tags:         nil, //Todo: is this ok?
		DroppedTrace: t.droppedTrace,
	}
}

func MakeTraces(payload *MsgpPayload) []*TableTraceChunk {
	st := StringTable{
		strings:       make([]*SharedString, len(payload.Strings)),
		stringIndexes: make(map[string]uint32, len(payload.Strings)),
	}
	for i, s := range payload.Strings {
		sharedStr := SharedString{
			s:    s,
			refs: 0,
		}
		st.strings[i] = &sharedStr
		st.stringIndexes[s] = uint32(i)
	}
	// Count a reference, returning the index (to make building a TableSpan read better)
	addRef := func(i uint32) uint32 {
		st.AddReference(i)
		return i
	}
	tableTraces := make([]*TableTraceChunk, len(payload.Traces))
	for i, payloadTrace := range payload.Traces {
		tableSpans := make([]*TableSpan, len(payloadTrace))
		for iSpan, payloadSpan := range payloadTrace {
			for k, v := range payloadSpan.Meta {
				st.AddReference(k)
				st.AddReference(v)
			}
			if payloadSpan.Meta == nil {
				payloadSpan.Meta = map[uint32]uint32{}
			}
			for k := range payloadSpan.Metrics {
				st.AddReference(k)
			}
			if payloadSpan.Metrics == nil {
				payloadSpan.Metrics = map[uint32]float64{}
			}
			ts := TableSpan{
				stringTable: &st,
				service:     addRef(payloadSpan.Service),
				name:        addRef(payloadSpan.Name),
				resource:    addRef(payloadSpan.Resource),
				traceID:     payloadSpan.TraceID,
				spanID:      payloadSpan.SpanID,
				parentID:    payloadSpan.ParentID,
				start:       payloadSpan.Start,
				duration:    payloadSpan.Duration,
				error:       payloadSpan.Error,
				meta:        payloadSpan.Meta,
				metrics:     payloadSpan.Metrics,
				typ:         addRef(payloadSpan.Typ),
			}
			tableSpans[iSpan] = &ts
		}
		tableTraces[i] = &TableTraceChunk{
			StringTable: &st,
			Spans:       tableSpans,
			priority:    math.MinInt8, //TODO: This is really sampler.PriorityNone but avoiding import cycle
			// Allow other fields to use default value (matches behavior of "traceChunksFromTraces")
		}
	}
	return tableTraces
}

// StringTable holds strings by index
// All usage assumes correct integer values
type StringTable struct {
	strings []*SharedString
	// Map of string to where it exists in `strings`
	stringIndexes map[string]uint32
}

func (st *StringTable) Strings() []string {
	strs := make([]string, len(st.strings))
	for i, sharedString := range st.strings {
		strs[i] = sharedString.s
	}
	return strs
}

func (st *StringTable) Get(i uint32) string {
	return st.strings[i].s
}

func (st *StringTable) GetIndex(s string) (uint32, bool) {
	i, ok := st.stringIndexes[s]
	return i, ok
}

// Update replaces the string at index i if the string is unshared,
// otherwise a new string is added and the new index is returned
func (st *StringTable) Update(i uint32, newS string) uint32 {
	oldS := st.strings[i]
	if oldS.refs > 1 {
		// More than one reference, add a new string
		// TODO: Should decrement ref here, but not strictly necessary for correctness (and will break the existing Add)
		return st.Add(newS) //TODO: can improve this by not double checking
	} else {
		st.strings[i].s = newS
		delete(st.stringIndexes, oldS.s)
		st.stringIndexes[newS] = i
		return i
	}
}

// Add appends s to the table if it's new otherwise increments the ref of the existing string
func (st *StringTable) Add(s string) uint32 {
	// If the string is already there, use it
	if sIdx, ok := st.stringIndexes[s]; ok {
		st.AddReference(sIdx)
		return sIdx
	}
	// If not, make a new entry
	st.strings = append(st.strings, &SharedString{
		s:    s,
		refs: 1,
	})
	newIndex := uint32(len(st.strings) - 1)
	st.stringIndexes[s] = newIndex
	return newIndex
}

// AddReference adds one ref count to the string at index i
func (st *StringTable) AddReference(i uint32) {
	st.strings[i].refs += 1
}

type SharedString struct {
	s    string
	refs int
}

type TableSpan struct {
	// string table is shared by whole trace
	stringTable *StringTable
	service     uint32
	name        uint32
	resource    uint32
	traceID     uint64
	spanID      uint64
	parentID    uint64
	start       int64
	duration    int64
	error       int32
	meta        map[uint32]uint32
	metrics     map[uint32]float64
	typ         uint32
}

func (t *TableSpan) TraceID() uint64 {
	return t.traceID
}

func (t *TableSpan) SpanID() uint64 {
	return t.spanID
}

func (t *TableSpan) ParentID() uint64 {
	return t.parentID
}

func (t *TableSpan) SetParentID(pid uint64) {
	t.parentID = pid
}

func (t *TableSpan) Duration() int64 {
	return t.duration
}

func (t *TableSpan) SetDuration(d int64) {
	t.duration = d
}

func (t *TableSpan) Start() int64 {
	return t.start
}

func (t *TableSpan) SetStart(s int64) {
	t.start = s
}

func (t *TableSpan) Service() string {
	return t.stringTable.Get(t.service)
}

func (t *TableSpan) SetService(s string) {
	t.service = t.stringTable.Update(t.service, s)
}

func (t *TableSpan) Name() string {
	return t.stringTable.Get(t.name)
}

func (t *TableSpan) SetName(n string) {
	t.name = t.stringTable.Update(t.name, n)
}

func (t *TableSpan) Resource() string {
	return t.stringTable.Get(t.resource)
}

func (t *TableSpan) SetResource(r string) {
	t.resource = t.stringTable.Update(t.resource, r)
}

func (t *TableSpan) Type() string {
	return t.stringTable.Get(t.typ)
}

func (t *TableSpan) SetType(ty string) {
	t.typ = t.stringTable.Update(t.typ, ty)
}

func (t *TableSpan) Error() int32 {
	return t.error
}

func (t *TableSpan) Meta(k string) (string, bool) {
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return "", false
	}
	vIdx, ok := t.meta[kIdx]
	if !ok {
		return "", false
	}
	return t.stringTable.Get(vIdx), true
}

func (t *TableSpan) ForMeta(f func(string, string) (string, string, bool)) bool {
	modified := false
	for kIdx, vIdx := range t.meta {
		k := t.stringTable.Get(kIdx)
		v := t.stringTable.Get(vIdx)
		newK, newV, shouldReplace := f(k, v)
		if shouldReplace {
			modified = true
			// TODO: Don't bother deleting from string table for now
			// It might be better, but also might be slower
			delete(t.meta, kIdx)
			newKIdx := t.stringTable.Update(kIdx, newK)
			newVIdx := t.stringTable.Update(vIdx, newV)
			t.meta[newKIdx] = newVIdx
		}
	}
	return modified
}

func (t *TableSpan) SetMeta(k string, v string) {
	// TODO: we can delete old entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		kIdx = t.stringTable.Add(k)
	}
	vIdx := t.stringTable.Add(v)
	t.meta[kIdx] = vIdx
}

func (t *TableSpan) DeleteMeta(k string) {
	// TODO: we can delete old string table entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return
	}
	delete(t.meta, kIdx)
}

func (t *TableSpan) Metrics(k string) (float64, bool) {
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return 0, false
	}
	v, ok := t.metrics[kIdx]
	return v, ok
}

func (t *TableSpan) SetMetrics(k string, v float64) {
	// TODO: we can delete old entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		kIdx = t.stringTable.Add(k)
	}
	t.metrics[kIdx] = v
}

func (t *TableSpan) ForMetrics(f func(string, float64) (string, float64, bool)) bool {
	modified := false
	for kIdx, v := range t.metrics {
		k := t.stringTable.Get(kIdx)
		newK, newV, shouldReplace := f(k, v)
		if shouldReplace {
			modified = true
			// TODO: Don't bother deleting from string table for now
			// It might be better, but also might be slower
			delete(t.meta, kIdx)
			newKIdx := t.stringTable.Update(kIdx, newK)
			t.metrics[newKIdx] = newV
		}
	}
	return modified
}

func (t *TableSpan) DeleteMetric(k string) {
	// TODO: we can delete old string table entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return
	}
	delete(t.metrics, kIdx)
}

func (t *TableSpan) ToPb() *pb.Span {
	meta := make(map[string]string, len(t.meta))
	for kIdx, vIdk := range t.meta {
		meta[t.stringTable.Get(kIdx)] = t.stringTable.Get(vIdk)
	}
	metrics := make(map[string]float64, len(t.metrics))
	for kIdx, v := range t.metrics {
		metrics[t.stringTable.Get(kIdx)] = v
	}
	return &pb.Span{
		Service:    t.Service(),
		Name:       t.Name(),
		Resource:   t.Resource(),
		TraceID:    t.traceID,
		SpanID:     t.spanID,
		ParentID:   t.parentID,
		Start:      t.start,
		Duration:   t.duration,
		Error:      t.error,
		Meta:       meta,
		Metrics:    metrics,
		Type:       t.Type(),
		MetaStruct: nil, //todo: is ok?
	}
}

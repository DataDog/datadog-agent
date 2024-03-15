package tracerpayload

import pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

type ProtoWrapped struct {
	TP *pb.TracerPayload
}

func (p *ProtoWrapped) SetTracerVersion(tv string) {
	p.TP.TracerVersion = tv
}

func (p *ProtoWrapped) SetContainerID(cid string) {
	p.TP.ContainerID = cid
}

func (p *ProtoWrapped) SetLanguageVersion(lv string) {
	p.TP.LanguageVersion = lv
}

type Generic interface {
	NumChunks() int
	Chunk(int) TraceChunk
	CloneChunks() []TraceChunk
	ReplaceChunk(int, TraceChunk)
	SetChunks([]TraceChunk)
	RemoveChunk(int)
	ContainerID() string
	SetTag(string, string)
	Env() string
	SetEnv(string)
	Hostname() string
	SetHostname(string)
	AppVersion() string
	SetAppVersion(string)
	SetLanguageName(string)
	Cut(i int) Generic
	ToPb() *pb.TracerPayload
	Serialize() ([]byte, error)
	SetLanguageVersion(string)
	SetContainerID(string)
	SetTracerVersion(string)
	IsCompressed() bool
}

func (p *ProtoWrapped) NumChunks() int {
	return len(p.TP.Chunks)
}

func (p *ProtoWrapped) Chunk(i int) TraceChunk {
	return &WrappedChunk{TC: p.TP.Chunks[i]}
}

func (p *ProtoWrapped) CloneChunks() []TraceChunk {
	clone := make([]TraceChunk, len(p.TP.Chunks))
	for i := 0; i < len(p.TP.Chunks); i++ {
		clone[i] = &WrappedChunk{TC: p.TP.Chunks[i]}
	}
	return clone
}

func (p *ProtoWrapped) ReplaceChunk(i int, newChunk TraceChunk) {
	nc := newChunk.(*WrappedChunk) //TODO: uhhh this won't always work OR MAYBE IT WILL!?
	p.TP.Chunks[i] = nc.TC
}

func (p *ProtoWrapped) SetChunks(newChunks []TraceChunk) {
	cs := make([]*pb.TraceChunk, len(newChunks))
	for i := 0; i < len(newChunks); i++ {
		wrappedChunk := newChunks[i].(*WrappedChunk) //TODO: uhhh this won't always work
		cs[i] = wrappedChunk.TC
	}
	p.TP.Chunks = cs
}

func (p *ProtoWrapped) RemoveChunk(i int) {
	p.TP.RemoveChunk(i)
}

func (p *ProtoWrapped) ContainerID() string {
	return p.TP.ContainerID
}

func (p *ProtoWrapped) Env() string {
	return p.TP.Env
}

func (p *ProtoWrapped) SetEnv(env string) {
	p.TP.Env = env
}

func (p *ProtoWrapped) Hostname() string {
	return p.TP.Hostname
}

func (p *ProtoWrapped) SetHostname(h string) {
	p.TP.Hostname = h
}

func (p *ProtoWrapped) AppVersion() string {
	return p.TP.AppVersion
}

func (p *ProtoWrapped) SetAppVersion(av string) {
	p.TP.AppVersion = av
}

func (p *ProtoWrapped) SetLanguageName(ln string) {
	p.TP.LanguageName = ln
}

func (p *ProtoWrapped) SetTag(key, value string) {
	if p.TP.Tags == nil {
		p.TP.Tags = make(map[string]string)
	}
	p.TP.Tags[key] = value
}

func (p *ProtoWrapped) Cut(i int) Generic {
	return &ProtoWrapped{TP: p.TP.Cut(i)}
}

func (p *ProtoWrapped) ToPb() *pb.TracerPayload {
	return p.TP
}

func (p *ProtoWrapped) Serialize() ([]byte, error) {
	bs, err := p.TP.MarshalVT()
	return bs, err
}

func (p *ProtoWrapped) IsCompressed() bool {
	return false
}

type WrappedChunk struct {
	TC *pb.TraceChunk
}

type TraceChunk interface {
	NumSpans() int
	Span(int) Span
	Priority() int32
	SetPriority(int32)
	Origin() string
	SetOrigin(string)
	DroppedTrace() bool
	SetDroppedTrace(bool)
	Msgsize() int
}

func (wc *WrappedChunk) NumSpans() int {
	return len(wc.TC.Spans)
}

func (wc *WrappedChunk) Span(i int) Span {
	return &WrappedSpan{s: wc.TC.Spans[i]}
}

func (wc *WrappedChunk) Priority() int32 {
	return wc.TC.Priority
}

func (wc *WrappedChunk) SetPriority(p int32) {
	wc.TC.Priority = p
}

func (wc *WrappedChunk) Origin() string {
	return wc.TC.Origin
}

func (wc *WrappedChunk) SetOrigin(o string) {
	wc.TC.Origin = o
}

func (wc *WrappedChunk) DroppedTrace() bool {
	return wc.TC.DroppedTrace
}

func (wc *WrappedChunk) SetDroppedTrace(dt bool) {
	wc.TC.DroppedTrace = dt
}

func (wc *WrappedChunk) Msgsize() int {
	return wc.TC.Msgsize()
}

type WrappedSpan struct {
	s *pb.Span
}

type Span interface {
	TraceID() uint64
	SpanID() uint64
	ParentID() uint64
	SetParentID(uint64)
	Duration() int64
	SetDuration(int64)
	Start() int64
	SetStart(int64)
	Service() string
	SetService(string)
	Name() string
	SetName(string)
	Resource() string
	SetResource(string)
	Type() string
	SetType(string)
	Error() int32
	Meta(string) (string, bool)
	ForMeta(func(string, string) (string, string, bool)) bool
	SetMeta(string, string)
	DeleteMeta(string)
	Metrics(string) (float64, bool)
	SetMetrics(string, float64)
	ForMetrics(func(string, float64) (string, float64, bool)) bool
	DeleteMetric(string)
}

func (s *WrappedSpan) TraceID() uint64 {
	return s.s.TraceID
}

func (s *WrappedSpan) SpanID() uint64 {
	return s.s.SpanID
}

func (s *WrappedSpan) ParentID() uint64 {
	return s.s.ParentID
}

func (s *WrappedSpan) SetParentID(parentID uint64) {
	s.s.ParentID = parentID
}

func (s *WrappedSpan) Duration() int64 {
	return s.s.Duration
}

func (s *WrappedSpan) SetDuration(duration int64) {
	s.s.Duration = duration
}

func (s *WrappedSpan) Start() int64 {
	return s.s.Start
}

func (s *WrappedSpan) SetStart(start int64) {
	s.s.Start = start
}

func (s *WrappedSpan) Service() string {
	return s.s.Service
}

func (s *WrappedSpan) SetService(service string) {
	s.s.Service = service
}

func (s *WrappedSpan) Name() string {
	return s.s.Name
}

func (s *WrappedSpan) SetName(name string) {
	s.s.Name = name
}

func (s *WrappedSpan) Resource() string {
	return s.s.Resource
}

func (s *WrappedSpan) SetResource(resource string) {
	s.s.Resource = resource
}

func (s *WrappedSpan) Type() string {
	return s.s.Type
}

func (s *WrappedSpan) SetType(typ string) {
	s.s.Type = typ
}

func (s *WrappedSpan) Error() int32 {
	return s.s.Error
}

func (s *WrappedSpan) Meta(key string) (string, bool) {
	v, ok := s.s.Meta[key]
	return v, ok
}

func (s *WrappedSpan) ForMeta(f func(string, string) (string, string, bool)) bool {
	modified := false
	for k, v := range s.s.Meta {
		newKey, newVal, shouldReplace := f(k, v)
		if shouldReplace {
			delete(s.s.Meta, k)
			s.s.Meta[newKey] = newVal
			modified = true
		}
	}
	return modified
}

func (s *WrappedSpan) SetMeta(key, val string) {
	s.s.Meta[key] = val
}

func (s *WrappedSpan) DeleteMeta(key string) {
	delete(s.s.Meta, key)
}

func (s *WrappedSpan) Metrics(key string) (float64, bool) {
	v, ok := s.s.Metrics[key]
	return v, ok
}

func (s *WrappedSpan) SetMetrics(key string, val float64) {
	s.s.Metrics[key] = val
}

func (s *WrappedSpan) ForMetrics(f func(string, float64) (string, float64, bool)) bool {
	modified := false
	for k, v := range s.s.Metrics {
		newKey, newVal, shouldReplace := f(k, v)
		if shouldReplace {
			delete(s.s.Metrics, k)
			s.s.Metrics[newKey] = newVal
			modified = true
		}
	}
	return modified
}

func (s *WrappedSpan) DeleteMetric(key string) {
	delete(s.s.Metrics, key)
}

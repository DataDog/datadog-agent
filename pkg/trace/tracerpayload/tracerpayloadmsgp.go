package tracerpayload

type MsgpPayload struct {
	_msgpack struct{} `msgpack:",as_array"`

	Strings []string
	Traces  [][]MsgpSpan
}

type MsgpSpan struct {
	_msgpack struct{} `msgpack:",as_array"`

	Service  uint32
	Name     uint32
	Resource uint32
	TraceID  uint64
	SpanID   uint64
	ParentID uint64
	Start    int64
	Duration int64
	Error    int32
	Meta     map[uint32]uint32
	Metrics  map[uint32]float64
	Typ      uint32 `msgpack:"Type"`
}

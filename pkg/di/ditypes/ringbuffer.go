package ditypes

type DIEvent struct {
	ProbeID  string
	PID      uint32
	UID      uint32
	Argdata  []*Param
	StackPCs []byte
}

type Param struct {
	ValueStr string `json:",omitempty"`
	Type     string
	Size     uint16
	Kind     byte
	Fields   []*Param `json:",omitempty"`
}

type StackFrame struct {
	FileName string `json:"fileName,omitempty"`
	Function string `json:"function,omitempty"`
	Line     int    `json:"lineNumber,omitempty"`
}

type EventCallback func(*DIEvent)

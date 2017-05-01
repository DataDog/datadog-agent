package gohai

type gohai struct {
	Cpu        interface{} `json:"cpu"`
	FileSystem interface{} `json:"filesystem"`
	Memory     interface{} `json:"memory"`
	Network    interface{} `json:"network"`
	Platform   interface{} `json:"platform"`
}
type Payload struct {
	Gohai *gohai `json:"gohai"`
}

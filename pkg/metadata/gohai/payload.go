package gohai

type gohai struct {
	CPU        interface{} `json:"cpu"`
	FileSystem interface{} `json:"filesystem"`
	Memory     interface{} `json:"memory"`
	Network    interface{} `json:"network"`
	Platform   interface{} `json:"platform"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Gohai *gohai `json:"gohai"`
}

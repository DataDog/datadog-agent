package resources

type Payload struct {
	Processes interface{}       `json:"processes"`
	Meta      map[string]string `json:"meta"`
}

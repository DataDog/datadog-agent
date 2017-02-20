package host

type systemStats struct {
	CPUCores  int32     `json:"cpuCores"`
	Machine   string    `json:"machine"`
	Platform  string    `json:"platform"`
	Pythonv   string    `json:"pythonV"`
	Processor string    `json:"processor"`
	Macver    osVersion `json:"macV"`
	Nixver    osVersion `json:"nixV"`
	Fbsdver   osVersion `json:"fbsdV"`
	Winver    osVersion `json:"winV"`
}

type meta struct {
	SocketHostname string   `json:"socket-hostname"`
	Timezones      []string `json:"timezones"`
	SocketFqdn     string   `json:"socket-fqdn"`
	EC2Hostname    string   `json:"ec2-hostname"`
	Hostname       string   `json:"hostname"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Os               string       `json:"os"`
	PythonVersion    string       `json:"python"`
	InternalHostname string       `json:"internalHostname"`
	UUID             string       `json:"uuid"`
	SytemStats       *systemStats `json:"systemStats"`
	Meta             *meta        `json:"meta"`
}

package remoteagent

type StatusRequest chan<- *StatusData
type StatusRequests <-chan StatusRequest

type FlareRequest chan<- *FlareData
type FlareRequests <-chan FlareRequest

type StatusSection map[string]string

type StatusData struct {
	AgentId       string
	FailureReason string
	MainSection   StatusSection
	NamedSections map[string]StatusSection
}

type FlareData struct {
	AgentId string
	Files   map[string][]byte
}

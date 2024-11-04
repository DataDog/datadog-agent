package haagent

type Integration struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type Payload struct {
	PrimaryAgent        string `json:"primary_agent"`
	ExpirationTimestamp int64  `json:"expiration_timestamp"`
}

package haagent

type Integration struct {
	ID   string
	Type string
}

type Payload struct {
	Integrations        []Integration `json:"integrations"` // TODO: change to list of check objects
	ExpirationTimestamp int64         `json:"expiration_timestamp"`
}

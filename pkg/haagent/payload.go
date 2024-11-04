package haagent

type Payload struct {
	PrimaryAgent        string `json:"primary_agent"`
	ExpirationTimestamp int64  `json:"expiration_timestamp"`
}

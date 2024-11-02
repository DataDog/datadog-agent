package haagent

type Payload struct {
	CheckIDs            []string `json:"check_ids"` // TODO: change to list of check objects
	ExpirationTimestamp int64    `json:"expiration_timestamp"`
}

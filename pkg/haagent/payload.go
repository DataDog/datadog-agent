package haagent

type Payload struct {
	Role                string   `json:"role"`
	CheckIDs            []string `json:"check_ids"`
	CollectTimestamp    int64    `json:"collect_timestamp"`
	ExpirationTimestamp int64    `json:"expiration_timestamp"`
}

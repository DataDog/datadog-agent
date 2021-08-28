package secret

type SecretOutput struct {
	Value *string `json:"value"`
	Error *string `json:"error"`
}

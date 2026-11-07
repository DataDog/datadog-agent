package protocol

// Payload limits shared by the client, connection and credit sizing paths.
const (
	maxDirectTCPSize = 0xffffff // 16777215

	maxSingleCreditPayloadSize = 64 * 1024
)

package metadata

// Payload is an interface shared by the output of the newer metadata providers.
// Right now this interface simply satifies the Protobuf interface.
type Payload interface {
	Reset()
	String() string
	ProtoMessage()
	Descriptor() ([]byte, []int)
}

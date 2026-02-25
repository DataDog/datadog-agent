package trace

import (
	"bytes"
	"testing"
)

type vtMarshaler interface {
	MarshalVT() ([]byte, error)
	MarshalToVT([]byte) (int, error)
	SizeVT() int
}

type vtUnmarshaler interface {
	UnmarshalVT([]byte) error
}

type typeFactory struct {
	name       string
	newMessage func() vtUnmarshaler
	seed       []byte
}

var typeFactories = []typeFactory{
	{
		name:       "Span",
		newMessage: func() vtUnmarshaler { return &Span{} },
		seed:       []byte{0x0a, 0x07, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65}, // service field
	},
	{
		name:       "SpanLink",
		newMessage: func() vtUnmarshaler { return &SpanLink{} },
		seed:       []byte{0x08, 0x01}, // TraceID field
	},
	{
		name:       "SpanEvent",
		newMessage: func() vtUnmarshaler { return &SpanEvent{} },
		seed:       []byte{0x12, 0x04, 0x74, 0x65, 0x73, 0x74}, // name field
	},
	{
		name:       "AttributeAnyValue",
		newMessage: func() vtUnmarshaler { return &AttributeAnyValue{} },
		seed:       []byte{0x08, 0x01}, // Type field
	},
	{
		name:       "AttributeArray",
		newMessage: func() vtUnmarshaler { return &AttributeArray{} },
		seed:       []byte{0x0a, 0x02, 0x08, 0x01}, // Values field with one element
	},
	{
		name:       "AttributeArrayValue",
		newMessage: func() vtUnmarshaler { return &AttributeArrayValue{} },
		seed:       []byte{0x08, 0x01}, // Type field
	},
}

func FuzzVTMarshalUnmarshal(f *testing.F) {
	// Add seed corpus: first byte is type selector, rest is payload
	f.Add([]byte{0}) // empty payload for type 0
	for i, factory := range typeFactories {
		seed := append([]byte{byte(i)}, factory.seed...)
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 1 {
			return
		}

		typeIdx := int(data[0]) % len(typeFactories)
		payload := data[1:]

		msg := typeFactories[typeIdx].newMessage()
		if err := msg.UnmarshalVT(payload); err != nil {
			return
		}

		marshaler := msg.(vtMarshaler)

		// Marshal the successfully unmarshalled message
		marshalled, err := marshaler.MarshalVT()
		if err != nil {
			t.Fatalf("[%s] MarshalVT failed after successful UnmarshalVT: %v",
				typeFactories[typeIdx].name, err)
		}

		// Unmarshal again to verify roundtrip
		msg2 := typeFactories[typeIdx].newMessage()
		if err := msg2.UnmarshalVT(marshalled); err != nil {
			t.Fatalf("[%s] UnmarshalVT failed on marshalled data: %v",
				typeFactories[typeIdx].name, err)
		}

		// Marshal again and compare bytes
		marshalled2, err := msg2.(vtMarshaler).MarshalVT()
		if err != nil {
			t.Fatalf("[%s] Second MarshalVT failed: %v",
				typeFactories[typeIdx].name, err)
		}

		if !bytes.Equal(marshalled, marshalled2) {
			t.Fatalf("[%s] Roundtrip mismatch: first marshal produced %d bytes, second produced %d bytes",
				typeFactories[typeIdx].name, len(marshalled), len(marshalled2))
		}
	})
}

func FuzzMarshalToVT(f *testing.F) {
	f.Add([]byte{0})
	for i, factory := range typeFactories {
		seed := append([]byte{byte(i)}, factory.seed...)
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 1 {
			return
		}

		typeIdx := int(data[0]) % len(typeFactories)
		// When crashing, this will print the type and payload that caused the crash for easier human consumption.
		t.Logf("Marshaller type: %s, payload: %v", typeFactories[typeIdx].name, data[1:])
		payload := data[1:]

		msg := typeFactories[typeIdx].newMessage()
		if err := msg.UnmarshalVT(payload); err != nil {
			return
		}

		marshaler := msg.(vtMarshaler)
		_, err := marshaler.MarshalToVT(data)
		if err != nil {
			return
		}
	})
}

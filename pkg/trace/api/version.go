package api

// Version is a dumb way to version our collector handlers
type Version string

const (
	// v01 DEPRECATED, FIXME[1.x]
	// Traces: JSON, slice of spans
	// Services: deprecated
	v01 Version = "v0.1"

	// v02 DEPRECATED, FIXME[1.x]
	// Traces: JSON, slice of traces
	// Services: deprecated
	v02 Version = "v0.2"

	// v03
	// Traces: msgpack/JSON (Content-Type) slice of traces
	// Services: deprecated
	v03 Version = "v0.3"

	// v04
	// Traces: msgpack/JSON (Content-Type) slice of traces + returns service sampling ratios
	// Services: deprecated
	v04 Version = "v0.4"

	// v05
	//
	// Content-Type: application/msgpack
	// Payload: Traces with strings de-duplicated into a dictionary.
	// Response: Service sampling rates.
	//
	// The payload is an array containing exactly 2 elements:
	//
	// 	1. An array of all unique strings present in the payload (a dictionary referred to by index).
	// 	2. An array of traces, where each trace is an array of spans. A span is encoded as an array having
	// 	   exactly 12 elements, representing all span properties, in this exact order:
	//
	// 		 0: Service   (int)
	// 		 1: Name      (int)
	// 		 2: Resource  (int)
	// 		 3: TraceID   (uint64)
	// 		 4: SpanID    (uint64)
	// 		 5: ParentID  (uint64)
	// 		 6: Start     (int64)
	// 		 7: Duration  (int64)
	// 		 8: Error     (int32)
	// 		 9: Meta      (map[int]int)
	// 		10: Metrics   (map[int]float64)
	// 		11: Type      (int)
	//
	// 	Considerations:
	//
	// 	- The "int" typed values represent the index at which the corresponding string is found in the dictionary.
	// 	  If any of the values are the empty string, then the empty string must be added into the dictionary.
	//
	// 	- None of the elements can be nil. If any of them are unset, they should be given their "zero-value". Here
	// 	  is an example of a span with all unset values:
	//
	// 		 0: 0                 // Service is "" (index 0 in dictionary)
	// 		 1: 0                 // Name is ""
	// 		 2: 0                 // Resource is ""
	// 		 3: 0                 // TraceID
	// 		 4: 0                 // SpanID
	// 		 5: 0                 // ParentID
	// 		 6: 0                 // Start
	// 		 7: 0                 // Duration
	// 		 8: 0                 // Error
	// 		 9: map[int]int{}     // Meta (empty map)
	// 		10: map[int]float64{} // Metrics (empty map)
	// 		11: 0                 // Type is ""
	//
	// 		The dictionary in this case would be []string{""}, having only the empty string at index 0.
	//
	v05 Version = "v0.5"
)

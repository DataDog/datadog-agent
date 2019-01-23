package info

// EndpointStats contains stats about the volume of data written
type EndpointStats struct {
	// TracesPayload is the number of traces payload sent, including errors.
	// If several URLs are given, each URL counts for one.
	TracesPayload int64
	// TracesPayloadError is the number of traces payload sent with an error.
	// If several URLs are given, each URL counts for one.
	TracesPayloadError int64
	// TracesBytes is the size of the traces payload data sent, including errors.
	// If several URLs are given, it does not change the size (shared for all).
	// This is the raw data, encoded, compressed.
	TracesBytes int64
	// TracesStats is the number of stats in the traces payload data sent, including errors.
	// If several URLs are given, it does not change the size (shared for all).
	TracesStats int64
	// TracesPayload is the number of services payload sent, including errors.
	// If several URLs are given, each URL counts for one.
	ServicesPayload int64
	// ServicesPayloadError is the number of services payload sent with an error.
	// If several URLs are given, each URL counts for one.
	ServicesPayloadError int64
	// TracesBytes is the size of the services payload data sent, including errors.
	// If several URLs are given, it does not change the size (shared for all).
	ServicesBytes int64
}

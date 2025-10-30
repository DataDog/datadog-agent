package proxy

// Network probes are intentionally stubbed for the MVP.
// They can be extended to perform DNS/TCP/TLS/HTTP checks when --no-network=false.

func ProbeProxyConnectivity(_ Effective) []Finding {
	return nil
}

func ProbeEndpointsConnectivity(_ Effective, _ []Endpoint) []Finding {
	return nil
}

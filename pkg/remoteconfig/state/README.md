# Remote Config Go client

This package powers the Remote Config client shipped in the Go tracer and in all the agent processes (core-agent, trace-agent, system-probe, ...).

To add a new product simply add it to `products.go` as a constant and in the `validProducts` set.

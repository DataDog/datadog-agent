module github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd

go 1.21.0

replace github.com/DataDog/datadog-agent/comp/trace/compression/def => ../../../../comp/trace/compression/def/

require (
	github.com/DataDog/datadog-agent/comp/trace/compression/def v0.0.0-00010101000000-000000000000
	github.com/DataDog/zstd v1.5.5
)

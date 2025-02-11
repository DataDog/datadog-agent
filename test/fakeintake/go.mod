module github.com/DataDog/datadog-agent/test/fakeintake

go 1.23.0

// every datadog-agent module replaced in the fakeintake go.mod needs to be copied in the Dockerfile
replace (
	github.com/DataDog/datadog-agent/comp/netflow/payload => ../../comp/netflow/payload
	github.com/DataDog/datadog-agent/pkg/network/payload => ../../pkg/network/payload
	github.com/DataDog/datadog-agent/pkg/networkpath/payload => ../../pkg/networkpath/payload
	github.com/DataDog/datadog-agent/pkg/proto => ../../pkg/proto
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.143
	github.com/DataDog/datadog-agent/comp/netflow/payload v0.56.0-rc.3
	github.com/DataDog/datadog-agent/pkg/networkpath/payload v0.0.0-20250128160050-7ac9ccd58c07
	github.com/DataDog/datadog-agent/pkg/proto v0.56.0-rc.3
	github.com/DataDog/zstd v1.5.6
	github.com/benbjohnson/clock v1.3.5
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/google/uuid v1.6.0
	github.com/kr/pretty v0.3.1
	github.com/olekukonko/tablewriter v0.0.5
	github.com/prometheus/client_golang v1.20.5
	github.com/samber/lo v1.47.0
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.10.0
	github.com/tinylib/msgp v1.2.5
	google.golang.org/protobuf v1.36.4
	modernc.org/sqlite v1.34.1
)

require (
	github.com/DataDog/datadog-agent/pkg/network/payload v0.0.0-20250128160050-7ac9ccd58c07 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/tools v0.29.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/gc/v3 v3.0.0-20240107210532-573471604cb6 // indirect
	modernc.org/libc v1.55.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/strutil v1.2.0 // indirect
	modernc.org/token v1.1.0 // indirect
)

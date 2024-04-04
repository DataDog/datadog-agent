module github.com/DataDog/datadog-agent/test/fakeintake

go 1.21.8

// every datadog-agent module replaced in the fakeintake go.mod needs to be copied in the Dockerfile
replace (
	github.com/DataDog/datadog-agent/comp/netflow/payload => ../../comp/netflow/payload
	github.com/DataDog/datadog-agent/pkg/proto => ../../pkg/proto
)

require (
	github.com/DataDog/agent-payload/v5 v5.0.106
	github.com/DataDog/datadog-agent/comp/netflow/payload v0.53.0-rc.2
	github.com/DataDog/datadog-agent/pkg/proto v0.53.0-rc.2
	github.com/benbjohnson/clock v1.3.5
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/kr/pretty v0.3.1
	github.com/olekukonko/tablewriter v0.0.5
	github.com/prometheus/client_golang v1.17.0
	github.com/samber/lo v1.39.0
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.9.0
	github.com/tinylib/msgp v1.1.8
	google.golang.org/protobuf v1.31.0
)

require (
	github.com/DataDog/mmh3 v0.0.0-20200805151601-30884ca2197a // indirect
	github.com/DataDog/zstd v1.4.8 // indirect
	github.com/DataDog/zstd_0 v0.0.0-20210310093942-586c1286621f // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-runewidth v0.0.9 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.4.1-0.20230718164431-9a2bf3000d16 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/exp v0.0.0-20220303212507-bbda1eaf7a17 // indirect
	golang.org/x/sys v0.15.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

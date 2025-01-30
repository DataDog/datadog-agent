module github.com/DataDog/datadog-agent/pkg/security/secl

go 1.23.0

replace github.com/DataDog/datadog-agent/pkg/util/jsonyaml => ../../util/jsonyaml

require (
	github.com/DataDog/datadog-agent/pkg/util/jsonyaml v0.0.0-00010101000000-000000000000
	github.com/Masterminds/semver/v3 v3.3.1
	github.com/alecthomas/participle v0.7.1
	github.com/google/go-cmp v0.6.0
	github.com/google/gopacket v1.1.19
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/jellydator/ttlcache/v3 v3.3.0
	github.com/skydive-project/go-debouncer v1.0.1
	github.com/spf13/cast v1.7.1
	github.com/stretchr/testify v1.10.0
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/sys v0.29.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
)

module github.com/DataDog/datadog-agent/pkg/delegatedauth

go 1.24.0

require (
	github.com/DataDog/datadog-agent/pkg/config/model v0.64.1
	github.com/DataDog/datadog-agent/pkg/config/utils v0.61.0
	github.com/DataDog/datadog-agent/pkg/util/ec2 v0.70.0
	github.com/DataDog/datadog-agent/pkg/util/http v0.70.0
	github.com/DataDog/datadog-agent/pkg/util/log v0.64.1
	github.com/DataDog/datadog-agent/pkg/version v0.64.1
	github.com/aws/aws-sdk-go-v2/aws v1.34.0
	github.com/aws/aws-sdk-go-v2/aws/signer/v4 v4.0.0
	github.com/stretchr/testify v1.11.1
)

replace (
	github.com/DataDog/datadog-agent/comp/api/api/def => ../../comp/api/api/def
	github.com/DataDog/datadog-agent/comp/core/flare/builder => ../../comp/core/flare/builder
	github.com/DataDog/datadog-agent/comp/core/flare/types => ../../comp/core/flare/types
	github.com/DataDog/datadog-agent/comp/core/secrets/def => ../../comp/core/secrets/def
	github.com/DataDog/datadog-agent/comp/core/status => ../../comp/core/status
	github.com/DataDog/datadog-agent/comp/core/tagger/def => ../../comp/core/tagger/def
	github.com/DataDog/datadog-agent/comp/core/tagger/types => ../../comp/core/tagger/types
	github.com/DataDog/datadog-agent/comp/core/telemetry => ../../comp/core/telemetry
	github.com/DataDog/datadog-agent/comp/def => ../../comp/def
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults => ../collector/check/defaults
	github.com/DataDog/datadog-agent/pkg/config/create => ../config/create
	github.com/DataDog/datadog-agent/pkg/config/env => ../config/env
	github.com/DataDog/datadog-agent/pkg/config/mock => ../config/mock
	github.com/DataDog/datadog-agent/pkg/config/model => ../config/model
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel => ../config/nodetreemodel
	github.com/DataDog/datadog-agent/pkg/config/setup => ../config/setup
	github.com/DataDog/datadog-agent/pkg/config/structure => ../config/structure
	github.com/DataDog/datadog-agent/pkg/config/teeconfig => ../config/teeconfig
	github.com/DataDog/datadog-agent/pkg/config/utils => ../config/utils
	github.com/DataDog/datadog-agent/pkg/config/viperconfig => ../config/viperconfig
	github.com/DataDog/datadog-agent/pkg/fips => ../fips
	github.com/DataDog/datadog-agent/pkg/template => ../template
	github.com/DataDog/datadog-agent/pkg/util/cache => ../util/cache
	github.com/DataDog/datadog-agent/pkg/util/common => ../util/common
	github.com/DataDog/datadog-agent/pkg/util/ec2 => ../util/ec2
	github.com/DataDog/datadog-agent/pkg/util/executable => ../util/executable
	github.com/DataDog/datadog-agent/pkg/util/filesystem => ../util/filesystem
	github.com/DataDog/datadog-agent/pkg/util/fxutil => ../util/fxutil
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate => ../util/hostname/validate
	github.com/DataDog/datadog-agent/pkg/util/http => ../util/http
	github.com/DataDog/datadog-agent/pkg/util/log => ../util/log
	github.com/DataDog/datadog-agent/pkg/util/option => ../util/option
	github.com/DataDog/datadog-agent/pkg/util/pointer => ../util/pointer
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../util/scrubber
	github.com/DataDog/datadog-agent/pkg/util/system => ../util/system
	github.com/DataDog/datadog-agent/pkg/util/system/socket => ../util/system/socket
	github.com/DataDog/datadog-agent/pkg/util/testutil => ../util/testutil
	github.com/DataDog/datadog-agent/pkg/util/winutil => ../util/winutil
	github.com/DataDog/datadog-agent/pkg/version => ../version
)

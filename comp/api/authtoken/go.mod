module github.com/DataDog/datadog-agent/comp/api/authtoken

go 1.22.0

replace (
	github.com/DataDog/datadog-agent/pkg/config/model => ../../../pkg/config/model
	github.com/DataDog/datadog-agent/pkg/util/log => ../../../pkg/util/log
	github.com/DataDog/datadog-agent/pkg/util/log/setup => ../../../pkg/util/log/setup
	github.com/DataDog/datadog-agent/pkg/util/scrubber => ../../../pkg/util/scrubber
)

require (
<<<<<<< HEAD
	github.com/DataDog/datadog-agent/comp/core/config v0.56.0
	github.com/DataDog/datadog-agent/comp/core/log/def v0.58.0-devel
	github.com/DataDog/datadog-agent/comp/core/log/mock v0.58.0-devel
	github.com/DataDog/datadog-agent/pkg/api v0.56.0
	github.com/DataDog/datadog-agent/pkg/util/fxutil v0.56.0
	github.com/DataDog/datadog-agent/pkg/util/optional v0.56.0
	github.com/stretchr/testify v1.9.0
	go.uber.org/fx v1.22.2
)

require (
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.56.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.56.0 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets v0.56.0 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/model v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/config/utils v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/hostname/validate v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.58.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.56.0 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.56.0 // indirect
	github.com/DataDog/viper v1.13.5 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
=======
	github.com/DataDog/datadog-agent/pkg/config/model v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.56.0-rc.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log/setup v0.58.0-devel // indirect
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.56.0-rc.3 // indirect
	github.com/DataDog/viper v1.13.5 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
>>>>>>> ef7f775d25 ([otel-agent] modularize comp/api/authtoken)
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
<<<<<<< HEAD
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/shirou/gopsutil/v3 v3.23.12 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/cobra v1.7.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.18.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.26.0 // indirect
	golang.org/x/exp v0.0.0-20240808152545-0cdaa3abc0fa // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
=======
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.5.1 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/sys v0.23.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
>>>>>>> ef7f775d25 ([otel-agent] modularize comp/api/authtoken)
)

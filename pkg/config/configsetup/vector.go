package configsetup

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/conf"
)

func bindVectorOptions(config conf.Config, datatype DataType) {
	config.BindEnvAndSetDefault(fmt.Sprintf("observability_pipelines_worker.%s.enabled", datatype), false)
	config.BindEnvAndSetDefault(fmt.Sprintf("observability_pipelines_worker.%s.url", datatype), "")

	config.BindEnvAndSetDefault(fmt.Sprintf("vector.%s.enabled", datatype), false)
	config.BindEnvAndSetDefault(fmt.Sprintf("vector.%s.url", datatype), "")
}

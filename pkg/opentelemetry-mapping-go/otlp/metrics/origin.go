// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metrics

import (
	"fmt"
	"strings"
)

// OriginProduct defines the origin product.
type OriginProduct int32

const (
	// OriginProductUnknown is the default origin product.
	OriginProductUnknown OriginProduct = 0
	// OriginProductDatadogAgent is the origin for metrics coming from the Datadog Agent OTLP Ingest.
	OriginProductDatadogAgent OriginProduct = 10
	// OriginProductDatadogExporter is the origin for metrics coming from the OpenTelemetry Collector Datadog Exporter.
	OriginProductDatadogExporter OriginProduct = 19
)

func (o OriginProduct) String() string {
	switch o {
	case OriginProductUnknown:
		return "unknown"
	case OriginProductDatadogAgent:
		return "datadog-agent"
	case OriginProductDatadogExporter:
		return "datadog-exporter"
	default:
		return fmt.Sprintf("OriginProduct(%d)", o)
	}
}

// OriginSubProduct defines the origin subproduct.
type OriginSubProduct int32

// OriginSubProductOTLP is the origin subproduct for all metrics coming from OTLP.
// All metrics produced by the translator MUST have origin subproduct set to OTLP.
const OriginSubProductOTLP OriginSubProduct = 17

func (o OriginSubProduct) String() string {
	switch o {
	case OriginSubProductOTLP:
		return "otlp"
	default:
		return fmt.Sprintf("OriginSubProduct(%d)", o)
	}
}

// OriginProductDetail defines the origin service.
type OriginProductDetail int32

// List all receivers that set the scope name.
const (
	OriginProductDetailUnknown                   OriginProductDetail = 0
	OriginProductDetailActiveDirectoryDSReceiver OriginProductDetail = 251
	OriginProductDetailAerospikeReceiver         OriginProductDetail = 252
	OriginProductDetailApacheReceiver            OriginProductDetail = 253
	OriginProductDetailApacheSparkReceiver       OriginProductDetail = 254
	OriginProductDetailAzureMonitorReceiver      OriginProductDetail = 255
	OriginProductDetailBigIPReceiver             OriginProductDetail = 256
	OriginProductDetailChronyReceiver            OriginProductDetail = 257
	OriginProductDetailCouchDBReceiver           OriginProductDetail = 258
	OriginProductDetailDockerStatsReceiver       OriginProductDetail = 217
	OriginProductDetailElasticsearchReceiver     OriginProductDetail = 218
	OriginProductDetailExpVarReceiver            OriginProductDetail = 219
	OriginProductDetailFileStatsReceiver         OriginProductDetail = 220
	OriginProductDetailFlinkMetricsReceiver      OriginProductDetail = 221
	OriginProductDetailGitProviderReceiver       OriginProductDetail = 222
	OriginProductDetailHAProxyReceiver           OriginProductDetail = 223
	OriginProductDetailHostMetricsReceiver       OriginProductDetail = 224
	OriginProductDetailHTTPCheckReceiver         OriginProductDetail = 225
	OriginProductDetailIISReceiver               OriginProductDetail = 226
	OriginProductDetailK8SClusterReceiver        OriginProductDetail = 227
	OriginProductDetailKafkaMetricsReceiver      OriginProductDetail = 228
	OriginProductDetailKubeletStatsReceiver      OriginProductDetail = 229
	OriginProductDetailMemcachedReceiver         OriginProductDetail = 230
	OriginProductDetailMongoDBAtlasReceiver      OriginProductDetail = 231
	OriginProductDetailMongoDBReceiver           OriginProductDetail = 232
	OriginProductDetailMySQLReceiver             OriginProductDetail = 233
	OriginProductDetailNginxReceiver             OriginProductDetail = 234
	OriginProductDetailNSXTReceiver              OriginProductDetail = 235
	OriginProductDetailOracleDBReceiver          OriginProductDetail = 236
	OriginProductDetailPostgreSQLReceiver        OriginProductDetail = 237
	OriginProductDetailPrometheusReceiver        OriginProductDetail = 238
	OriginProductDetailRabbitMQReceiver          OriginProductDetail = 239
	OriginProductDetailRedisReceiver             OriginProductDetail = 240
	OriginProductDetailRiakReceiver              OriginProductDetail = 241
	OriginProductDetailSAPHANAReceiver           OriginProductDetail = 242
	OriginProductDetailSNMPReceiver              OriginProductDetail = 243
	OriginProductDetailSnowflakeReceiver         OriginProductDetail = 244
	OriginProductDetailSplunkEnterpriseReceiver  OriginProductDetail = 245
	OriginProductDetailSQLServerReceiver         OriginProductDetail = 246
	OriginProductDetailSSHCheckReceiver          OriginProductDetail = 247
	OriginProductDetailStatsDReceiver            OriginProductDetail = 248
	OriginProductDetailVCenterReceiver           OriginProductDetail = 249
	OriginProductDetailZookeeperReceiver         OriginProductDetail = 250
)

func originProductDetailFromScopeName(scopeName string) OriginProductDetail {
	const collectorPrefix = "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/"
	if !strings.HasPrefix(scopeName, collectorPrefix) {
		return OriginProductDetailUnknown
	}

	// github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver -> kubeletstatsreceiver
	// github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/disk -> hostmetricsreceiver
	receiverName := strings.Split(scopeName, "/")[4]

	// otelcol
	switch receiverName {
	case "activedirectorydsreceiver":
		return OriginProductDetailActiveDirectoryDSReceiver
	case "aerospikereceiver":
		return OriginProductDetailAerospikeReceiver
	case "apachereceiver":
		return OriginProductDetailApacheReceiver
	case "apachesparkreceiver":
		return OriginProductDetailApacheSparkReceiver
	case "azuremonitorreceiver":
		return OriginProductDetailAzureMonitorReceiver
	case "bigipreceiver":
		return OriginProductDetailBigIPReceiver
	case "chronyreceiver":
		return OriginProductDetailChronyReceiver
	case "couchdbreceiver":
		return OriginProductDetailCouchDBReceiver
	case "dockerstatsreceiver":
		return OriginProductDetailDockerStatsReceiver
	case "elasticsearchreceiver":
		return OriginProductDetailElasticsearchReceiver
	case "expvarreceiver":
		return OriginProductDetailExpVarReceiver
	case "filestatsreceiver":
		return OriginProductDetailFileStatsReceiver
	case "flinkmetricsreceiver":
		return OriginProductDetailFlinkMetricsReceiver
	case "gitproviderreceiver":
		return OriginProductDetailGitProviderReceiver
	case "haproxyreceiver":
		return OriginProductDetailHAProxyReceiver
	case "hostmetricsreceiver":
		return OriginProductDetailHostMetricsReceiver
	case "httpcheckreceiver":
		return OriginProductDetailHTTPCheckReceiver
	case "iisreceiver":
		return OriginProductDetailIISReceiver
	case "k8sclusterreceiver":
		return OriginProductDetailK8SClusterReceiver
	case "kafkametricsreceiver":
		return OriginProductDetailKafkaMetricsReceiver
	case "kubeletstatsreceiver":
		return OriginProductDetailKubeletStatsReceiver
	case "memcachedreceiver":
		return OriginProductDetailMemcachedReceiver
	case "mongodbatlasreceiver":
		return OriginProductDetailMongoDBAtlasReceiver
	case "mongodbreceiver":
		return OriginProductDetailMongoDBReceiver
	case "mysqlreceiver":
		return OriginProductDetailMySQLReceiver
	case "nginxreceiver":
		return OriginProductDetailNginxReceiver
	case "nsxtreceiver":
		return OriginProductDetailNSXTReceiver
	case "oracledbreceiver":
		return OriginProductDetailOracleDBReceiver
	case "postgresqlreceiver":
		return OriginProductDetailPostgreSQLReceiver
	case "prometheusreceiver":
		return OriginProductDetailPrometheusReceiver
	case "rabbitmqreceiver":
		return OriginProductDetailRabbitMQReceiver
	case "redisreceiver":
		return OriginProductDetailRedisReceiver
	case "riakreceiver":
		return OriginProductDetailRiakReceiver
	case "saphanareceiver":
		return OriginProductDetailSAPHANAReceiver
	case "snmpreceiver":
		return OriginProductDetailSNMPReceiver
	case "snowflakereceiver":
		return OriginProductDetailSnowflakeReceiver
	case "splunkenterprisereceiver":
		return OriginProductDetailSplunkEnterpriseReceiver
	case "sqlserverreceiver":
		return OriginProductDetailSQLServerReceiver
	case "sshcheckreceiver":
		return OriginProductDetailSSHCheckReceiver
	case "statsdreceiver":
		return OriginProductDetailStatsDReceiver
	case "vcenterreceiver":
		return OriginProductDetailVCenterReceiver
	case "zookeeperreceiver":
		return OriginProductDetailZookeeperReceiver
	}

	return OriginProductDetailUnknown
}

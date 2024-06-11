module github.com/datadog-agent/DataDog/comp/otelcol/collector-contrib/impl

go 1.21.0

replace github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def => ../def

require (
	github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/def v0.54.1-rc.1
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/countconnector v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/exceptionsconnector v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/grafanacloudconnector v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/routingconnector v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/fileexporter v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/signalfxexporter v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/splunkhecexporter v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/ackextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/asapauthextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/awsproxy v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/bearertokenauthextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/headerssetterextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/httpforwarderextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/jaegerremotesampling v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/oauth2clientauthextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/dockerobserver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecsobserver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/ecstaskobserver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/hostobserver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer/k8sobserver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/oidcauthextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/opampextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/sigv4authextension v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/dbstorage v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/filestorage v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/cumulativetodeltaprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatorateprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/groupbyattrsprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/groupbytraceprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/metricsgenerationprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/metricstransformprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/probabilisticsamplerprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/redactionprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/remotetapprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/spanprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/sumologicprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/activedirectorydsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/aerospikereceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/apachereceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/apachesparkreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awsecscontainermetricsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awsfirehosereceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awsxrayreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/azureblobreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/azureeventhubreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/azuremonitorreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/bigipreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/carbonreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/chronyreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/cloudflarereceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/cloudfoundryreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/collectdreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/couchdbreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/dockerstatsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/elasticsearchreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/expvarreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filestatsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/flinkmetricsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/fluentforwardreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/googlecloudpubsubreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/googlecloudspannerreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/haproxyreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/httpcheckreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/iisreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/influxdbreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/jmxreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/journaldreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8seventsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sobjectsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkametricsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kubeletstatsreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/memcachedreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mongodbatlasreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mongodbreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/mysqlreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/namedpipereceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/nginxreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/nsxtreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/opencensusreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/oracledbreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/otlpjsonfilereceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/podmanreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/postgresqlreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/pulsarreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/purefareceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/purefbreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/rabbitmqreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/receivercreator v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/redisreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/riakreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/sapmreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/signalfxreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/skywalkingreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/snmpreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/solacereceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/splunkhecreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/sqlserverreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/sshcheckreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/statsdreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/syslogreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/tcplogreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/udplogreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/vcenterreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/wavefrontreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/webhookeventreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/windowseventlogreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/windowsperfcountersreceiver v0.98.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/zookeeperreceiver v0.98.0
	go.opentelemetry.io/collector/connector v0.99.0
	go.opentelemetry.io/collector/connector/forwardconnector v0.98.0
	go.opentelemetry.io/collector/exporter v0.99.0
	go.opentelemetry.io/collector/exporter/debugexporter v0.98.0
	go.opentelemetry.io/collector/exporter/otlpexporter v0.98.0
	go.opentelemetry.io/collector/exporter/otlphttpexporter v0.95.0
	go.opentelemetry.io/collector/extension v0.99.0
	go.opentelemetry.io/collector/extension/ballastextension v0.98.0
	go.opentelemetry.io/collector/extension/zpagesextension v0.99.0
	go.opentelemetry.io/collector/otelcol v0.99.0
	go.opentelemetry.io/collector/processor v0.99.0
	go.opentelemetry.io/collector/processor/batchprocessor v0.98.0
	go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.98.0
	go.opentelemetry.io/collector/receiver v0.99.0
	go.opentelemetry.io/collector/receiver/nopreceiver v0.98.0
	go.opentelemetry.io/collector/receiver/otlpreceiver v0.98.0
	go.uber.org/multierr v1.11.0
)

require (
	bitbucket.org/atlassian/go-asap/v2 v2.8.0 // indirect
	cloud.google.com/go v0.112.1 // indirect
	cloud.google.com/go/compute v1.24.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v1.1.6 // indirect
	cloud.google.com/go/logging v1.9.0 // indirect
	cloud.google.com/go/longrunning v0.5.5 // indirect
	cloud.google.com/go/pubsub v1.37.0 // indirect
	cloud.google.com/go/spanner v1.60.0 // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20211115184647-b584dd5df32c // indirect
	code.cloudfoundry.org/go-loggregator v7.4.0+incompatible // indirect
	code.cloudfoundry.org/rfc5424 v0.0.0-20201103192249-000122071b78 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4 // indirect
	github.com/99designs/keyring v1.2.2 // indirect
	github.com/AthenZ/athenz v1.10.39 // indirect
	github.com/Azure/azure-amqp-common-go/v4 v4.2.0 // indirect
	github.com/Azure/azure-event-hubs-go/v3 v3.6.2 // indirect
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.11.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.5.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.5.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.5.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor v0.11.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.3.2 // indirect
	github.com/Azure/go-amqp v1.0.5 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.28 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.21 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/Code-Hex/go-generics-cache v1.3.1 // indirect
	github.com/DataDog/zstd v1.5.0 // indirect
	github.com/GehirnInc/crypt v0.0.0-20200316065508-bb7000b8a962 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.22.0 // indirect
	github.com/IBM/sarama v1.43.1 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ReneKroon/ttlcache/v2 v2.11.0 // indirect
	github.com/SermoDigital/jose v0.9.2-0.20180104203859-803625baeddc // indirect
	github.com/Showmax/go-fqdn v1.0.0 // indirect
	github.com/aerospike/aerospike-client-go/v6 v6.13.0 // indirect
	github.com/alecthomas/participle/v2 v2.1.1 // indirect
	github.com/alecthomas/units v0.0.0-20231202071711-9a357b53e9c9 // indirect
	github.com/apache/pulsar-client-go v0.8.1 // indirect
	github.com/apache/pulsar-client-go/oauth2 v0.0.0-20220120090717-25e59572242e // indirect
	github.com/apache/thrift v0.20.0 // indirect
	github.com/ardielle/ardielle-go v1.5.2 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/aws/aws-sdk-go v1.51.17 // indirect
	github.com/aws/aws-sdk-go-v2 v1.26.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.27.11 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.11 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.5 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.11.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.20.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.23.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.28.6 // indirect
	github.com/aws/smithy-go v1.20.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmatcuk/doublestar/v4 v4.6.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudfoundry-incubator/uaago v0.0.0-20190307164349-8136b7bbe76e // indirect
	github.com/cncf/xds/go v0.0.0-20231128003011-0fa0005c9caa // indirect
	github.com/coreos/go-oidc/v3 v3.10.0 // indirect
	github.com/danieljoos/wincred v1.1.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/devigned/tab v0.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/digitalocean/godo v1.109.0 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/docker v25.0.5+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dvsekhvalnov/jose2go v1.6.0 // indirect
	github.com/eapache/go-resiliency v1.6.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20230731223053-c322873962e3 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/envoyproxy/go-control-plane v0.12.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.0.4 // indirect
	github.com/expr-lang/expr v1.16.3 // indirect
	github.com/facebook/time v0.0.0-20240109160331-d1456d1a6bac // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-jose/go-jose/v4 v4.0.1 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/jsonreference v0.20.4 // indirect
	github.com/go-openapi/swag v0.22.9 // indirect
	github.com/go-resty/resty/v2 v2.11.0 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.0.0-alpha.1 // indirect
	github.com/go-zookeeper/zk v1.0.3 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/godbus/dbus v0.0.0-20190726142602-4481cbc300e2 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.3 // indirect
	github.com/gophercloud/gophercloud v1.8.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/gosnmp/gosnmp v1.37.0 // indirect
	github.com/grafana/regexp v0.0.0-20221122212121-6b5c0a4cb7fd // indirect
	github.com/grobie/gomemcache v0.0.0-20230213081705-239240bbc445 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.1 // indirect
	github.com/gsterjov/go-libsecret v0.0.0-20161001094733-a6f4afe4910c // indirect
	github.com/hashicorp/consul/api v1.28.2 // indirect
	github.com/hashicorp/cronexpr v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/nomad/api v0.0.0-20240306004928-3e7191ccb702 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/hetznercloud/hcloud-go/v2 v2.6.0 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/influxdata/go-syslog/v3 v3.0.1-0.20230911200830-875f5bc594a4 // indirect
	github.com/influxdata/influxdb-observability/common v0.5.8 // indirect
	github.com/influxdata/influxdb-observability/influx2otel v0.5.8 // indirect
	github.com/influxdata/line-protocol/v2 v2.2.1 // indirect
	github.com/ionos-cloud/sdk-go/v6 v6.1.11 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.14.3 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.3.3 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgtype v1.14.0 // indirect
	github.com/jackc/pgx/v4 v4.18.3 // indirect
	github.com/jaegertracing/jaeger v1.55.0 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.1 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/leodido/ragel-machinery v0.0.0-20181214104525-299bdde78165 // indirect
	github.com/leoluk/perflib_exporter v0.2.1 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/lightstep/go-expohisto v1.0.0 // indirect
	github.com/linkedin/goavro/v2 v2.9.8 // indirect
	github.com/linode/linodego v1.30.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/miekg/dns v1.1.58 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/hashstructure v1.1.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/mapstructure v1.5.1-0.20231216201459-8508981c8b6c // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mongodb-forks/digest v1.1.0 // indirect
	github.com/montanaflynn/stats v0.7.0 // indirect
	github.com/mostynb/go-grpc-compression v1.2.2 // indirect
	github.com/mtibben/percent v0.2.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/nginxinc/nginx-prometheus-exporter v0.11.0 // indirect
	github.com/oklog/ulid/v2 v2.1.0 // indirect
	github.com/open-telemetry/opamp-go v0.14.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/extension/observer v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/aws/awsutil v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/aws/ecsutil v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/aws/proxy v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/aws/xray v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/collectd v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/coreinternal v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/docker v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/filter v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/k8sconfig v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/kafka v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/kubelet v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/metadataproviders v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/sharedcomponent v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/splunk v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/batchperresourceattr v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/batchpersignal v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/experimentalmetricmetadata v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/azure v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/opencensus v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/prometheus v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/signalfx v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/skywalking v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/zipkin v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/winperfcounters v0.98.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.98.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/openshift/api v3.9.0+incompatible // indirect
	github.com/openshift/client-go v0.0.0-20210521082421-73d9475a9142 // indirect
	github.com/openzipkin/zipkin-go v0.4.2 // indirect
	github.com/ovh/go-ovh v1.4.3 // indirect
	github.com/pelletier/go-toml/v2 v2.1.0 // indirect
	github.com/philhofer/fwd v1.1.2 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/sftp v1.13.6 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/pquerna/cachecontrol v0.1.0 // indirect
	github.com/prometheus/client_golang v1.19.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.52.3 // indirect
	github.com/prometheus/common/sigv4 v0.1.0 // indirect
	github.com/prometheus/procfs v0.13.0 // indirect
	github.com/prometheus/prometheus v0.51.2-0.20240405174432-b4a973753c6e // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/redis/go-redis/v9 v9.5.1 // indirect
	github.com/relvacode/iso8601 v1.4.0 // indirect
	github.com/rs/cors v1.10.1 // indirect
	github.com/sagikazarmark/locafero v0.4.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.25 // indirect
	github.com/shirou/gopsutil/v3 v3.24.3 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/signalfx/com_signalfx_metrics_protobuf v0.0.3 // indirect
	github.com/signalfx/sapm-proto v0.14.0 // indirect
	github.com/sijms/go-ora/v2 v2.8.11 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/soheilhy/cmux v0.1.5 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/afero v1.11.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/cobra v1.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.18.2 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tg123/go-htpasswd v1.2.2 // indirect
	github.com/tidwall/gjson v1.14.2 // indirect
	github.com/tilinna/clock v1.1.0 // indirect
	github.com/tinylib/msgp v1.1.9 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/valyala/fastjson v1.6.4 // indirect
	github.com/vincent-petithory/dataurl v1.0.0 // indirect
	github.com/vmware/go-vmware-nsxt v0.0.0-20230223012718-d31b8a1ca05e // indirect
	github.com/vmware/govmomi v0.36.3 // indirect
	github.com/vultr/govultr/v2 v2.17.2 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20181117223130-1be2e3e5546d // indirect
	github.com/yuin/gopher-lua v0.0.0-20220504180219-658193537a64 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.etcd.io/bbolt v1.3.9 // indirect
	go.mongodb.org/atlas v0.36.0 // indirect
	go.mongodb.org/mongo-driver v1.14.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector v0.99.0 // indirect
	go.opentelemetry.io/collector/component v0.99.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.98.0 // indirect
	go.opentelemetry.io/collector/config/configcompression v1.5.0 // indirect
	go.opentelemetry.io/collector/config/configgrpc v0.98.0 // indirect
	go.opentelemetry.io/collector/config/confighttp v0.98.0 // indirect
	go.opentelemetry.io/collector/config/confignet v0.99.0 // indirect
	go.opentelemetry.io/collector/config/configopaque v1.5.0 // indirect
	go.opentelemetry.io/collector/config/configretry v0.99.0 // indirect
	go.opentelemetry.io/collector/config/configtelemetry v0.99.0 // indirect
	go.opentelemetry.io/collector/config/configtls v0.98.0 // indirect
	go.opentelemetry.io/collector/config/internal v0.98.0 // indirect
	go.opentelemetry.io/collector/confmap v0.99.0 // indirect
	go.opentelemetry.io/collector/confmap/converter/expandconverter v0.99.0 // indirect
	go.opentelemetry.io/collector/confmap/provider/envprovider v0.99.0 // indirect
	go.opentelemetry.io/collector/confmap/provider/fileprovider v0.99.0 // indirect
	go.opentelemetry.io/collector/confmap/provider/httpprovider v0.99.0 // indirect
	go.opentelemetry.io/collector/confmap/provider/httpsprovider v0.99.0 // indirect
	go.opentelemetry.io/collector/confmap/provider/yamlprovider v0.99.0 // indirect
	go.opentelemetry.io/collector/consumer v0.99.0 // indirect
	go.opentelemetry.io/collector/extension/auth v0.98.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.6.0 // indirect
	go.opentelemetry.io/collector/pdata v1.6.0 // indirect
	go.opentelemetry.io/collector/semconv v0.99.0 // indirect
	go.opentelemetry.io/collector/service v0.99.0 // indirect
	go.opentelemetry.io/contrib/config v0.5.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.49.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.25.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.50.0 // indirect
	go.opentelemetry.io/otel v1.25.0 // indirect
	go.opentelemetry.io/otel/bridge/opencensus v1.25.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.25.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.25.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.25.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.25.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.25.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.47.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.25.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.25.0 // indirect
	go.opentelemetry.io/otel/metric v1.25.0 // indirect
	go.opentelemetry.io/otel/sdk v1.25.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.25.0 // indirect
	go.opentelemetry.io/otel/trace v1.25.0 // indirect
	go.opentelemetry.io/proto/otlp v1.2.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.22.0 // indirect
	golang.org/x/exp v0.0.0-20240205201215-2c58cdc269a3 // indirect
	golang.org/x/mod v0.16.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/oauth2 v0.18.0 // indirect
	golang.org/x/sync v0.6.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/term v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	golang.org/x/tools v0.19.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	gonum.org/v1/gonum v0.15.0 // indirect
	google.golang.org/api v0.172.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto v0.0.0-20240227224415-6ceb2ff114de // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240318140521-94a12d6c2237 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240401170217-c3f982113cda // indirect
	google.golang.org/grpc v1.63.2 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/api v0.29.3 // indirect
	k8s.io/apimachinery v0.29.3 // indirect
	k8s.io/client-go v0.29.3 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	k8s.io/kubelet v0.29.3 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
	skywalking.apache.org/repo/goapi v0.0.0-20240104145220-ba7202308dd4 // indirect
)

package logs

import (
	"io/ioutil"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	serverlessMetric "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogsCollection is the route on which the AWS environment is sending the logs
// for the extension to collect them. It is attached to the main HTTP server
// already receiving hits from the libraries client.
type LogsCollection struct {
	LogChannel    chan *logConfig.ChannelMessage
	MetricChannel chan []metrics.MetricSample
	ExtraTags     []string
}

// ServeHTTP - see type LogsCollection comment.
func (l *LogsCollection) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	data, _ := ioutil.ReadAll(r.Body)
	defer r.Body.Close()

	messages, err := aws.ParseLogsAPIPayload(data)
	if err != nil {
		w.WriteHeader(400)
	} else {
		processLogMessages(l, messages)
		w.WriteHeader(200)
	}
}

func processLogMessages(l *LogsCollection, messages []aws.LogMessage) {
	metricTags := serverlessMetric.AddColdStartTag(l.ExtraTags)
	logsEnabled := config.Datadog.GetBool("serverless.logs_enabled")
	enhancedMetricsEnabled := config.Datadog.GetBool("enhanced_metrics")
	arn := aws.GetARN()
	lastRequestID := aws.GetRequestID()
	for _, message := range messages {
		processMessage(message, arn, lastRequestID, enhancedMetricsEnabled, metricTags, l.MetricChannel)
		// We always collect and process logs for the purpose of extracting enhanced metrics.
		// However, if logs are not enabled, we do not send them to the intake.
		if logsEnabled {
			logMessage := logConfig.NewChannelMessageFromLambda([]byte(message.StringRecord), message.Time, arn, lastRequestID)
			l.LogChannel <- logMessage
		}
	}
}

// processMessage performs logic about metrics and tags on the message
func processMessage(message aws.LogMessage, arn string, lastRequestID string, computeEnhancedMetrics bool, metricTags []string, metricsChan chan []metrics.MetricSample) {
	// Do not send logs or metrics if we can't associate them with an ARN or Request ID
	// First, if the log has a Request ID, set the global Request ID variable
	if message.Type == aws.LogTypePlatformStart {
		if len(message.ObjectRecord.RequestID) > 0 {
			aws.SetRequestID(message.ObjectRecord.RequestID)
			lastRequestID = message.ObjectRecord.RequestID
		}
	}

	if !aws.ShouldProcessLog(arn, lastRequestID, message) {
		return
	}

	if computeEnhancedMetrics {
		serverlessMetric.GenerateEnhancedMetrics(message, metricTags, metricsChan)
	}

	switch message.Type {
	case aws.LogTypePlatformReport:
		aws.SetColdStart(false)
	case aws.LogTypePlatformLogsDropped:
		log.Debug("Logs were dropped by the AWS Lambda Logs API")
	}
}

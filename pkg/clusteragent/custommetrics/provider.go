package custommetrics

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	//"github.com/golang/glog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/hpa"
)

type externalMetric struct {
	info  provider.ExternalMetricInfo
	value external_metrics.ExternalMetricValue
}

type datadogProvider struct {
	client dynamic.ClientPool
	mapper apimeta.RESTMapper

	values          map[provider.CustomMetricInfo]int64
	externalMetrics []externalMetric
	resVersion      string
}

// NewDatadogProvider creates a Custom Metrics and External Metrics Provider.
func NewDatadogProvider(client dynamic.ClientPool, mapper apimeta.RESTMapper) provider.MetricsProvider {
	return &datadogProvider{
		client: client,
		mapper: mapper,
		values: make(map[provider.CustomMetricInfo]int64),
	}
}

func (p *datadogProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("Not Implemented - RootScopedByName")
}

func (p *datadogProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("Not Implemented - RootScopedBySelector")
}

func (p *datadogProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("Not Implemented - NamespacedMetricByName")
}

func (p *datadogProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("Not Implemented - NamespacedMetricBySelector")
}

func (p *datadogProvider) ListAllMetrics() []provider.CustomMetricInfo {
	//// ListAllMetrics Will read from a ConfigMap, similarly to ListExternalMetrics
	return nil
}

// ListAllExternalMetrics is called every 30 seconds, although this is configurable on the API Server's end.
func (p *datadogProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	cl := hpa.GetHPAWatcherClient()
	rawMetrics := cl.ReadConfigMap()

	externalMetricsInfoList := []provider.ExternalMetricInfo{}
	externalMetricsList := []externalMetric{}

	for _, metric := range rawMetrics {
		var extMetric externalMetric
		extMetric.info = provider.ExternalMetricInfo{
			Metric: metric.Name,
			Labels: metric.Labels,
		}
		extMetric.value = external_metrics.ExternalMetricValue{
			MetricName:   metric.Name,
			MetricLabels: metric.Labels,
			Value:        *resource.NewQuantity(metric.Value, resource.DecimalSI),
		}
		externalMetricsList = append(externalMetricsList, extMetric)

		externalMetricsInfoList = append(externalMetricsInfoList, provider.ExternalMetricInfo{
			Metric: metric.Name,
			Labels: metric.Labels,
		})
	}
	p.externalMetrics = externalMetricsList
	log.Tracef("ListAllExternalMetrics returns: %#v", externalMetricsInfoList)
	return externalMetricsInfoList
}

// GetExternalMetric is called every 30 seconds as a result of:
// - The registering of the External Metrics Provider
// - The creation of a HPA manifest with an External metrics type.
// - The validation of the metrics against Datadog
func (p *datadogProvider) GetExternalMetric(namespace string, metricName string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	matchingMetrics := []external_metrics.ExternalMetricValue{}
	for _, metric := range p.externalMetrics {

		metricFromDatadog := external_metrics.ExternalMetricValue{
			MetricName:   metricName,
			MetricLabels: metric.info.Labels,
			Value:        metric.value.Value,
		}
		if metric.info.Metric == metricName &&
			metricSelector.Matches(labels.Set(metric.info.Labels)) {
			metricValue := metricFromDatadog
			metricValue.Timestamp = metav1.Now()
			matchingMetrics = append(matchingMetrics, metricValue)
		}
	}
	return &external_metrics.ExternalMetricValueList{
		Items: matchingMetrics,
	}, nil
}

package custommetrics

import (
	"fmt"
	"io/ioutil"
	"time"

	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	//apierr "k8s.io/apimachinery/pkg/api/errors"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	//"github.com/golang/glog"
	"github.com/cihub/seelog"
	"gopkg.in/yaml.v2"
)

type datadogProvider struct {
	client dynamic.ClientPool
}

type config struct {
	Metrics []configuredMetric `yaml:"metrics"`
}

type configuredMetric struct {
	Name  string `yaml:"name,omitempty"`
	Value int64  `yaml:"value,omitempty"`
}

func NewDatadogProvider(client dynamic.ClientPool) provider.CustomMetricsProvider {
	return &datadogProvider{
		client: client,
	}
}

func (p *datadogProvider) getValueFromFile(metricName string) (int64, error) {
	fileContents, err := ioutil.ReadFile("/opt/datadog-agent/dev/dist/metrics.yaml")
	if err != nil {
		return 0, fmt.Errorf("error reading metrics file at /opt/datadog-agent/dev/dist/metrics.yaml: %s", err)
	}
	unmarshalledConfig := config{}
	err = yaml.Unmarshal(fileContents, &unmarshalledConfig)
	if err != nil {
		return 0, fmt.Errorf("error unmarshalling metrics file: %s", err)
	}
	for _, metric := range unmarshalledConfig.Metrics {
		if metric.Name == metricName {
			if metric.Value == 0 {
				return 0, fmt.Errorf("found value 0 in metrics file for %s, ignoring: %s", metricName, err)
			}
			return metric.Value, nil
		}
	}

	return 0, fmt.Errorf("Could not find metric name %s in conf file: %s", metricName, err)
}

func (p *datadogProvider) getValue(metricName string) (int64, error) {
	value, err := p.getValueFromFile(metricName)
	if err != nil {
		seelog.Info("Metric %s value is not in file, querying API", metricName)

		var err error
		value, err = queryDatadog(metricName)
		if err != nil {
			return 0, err
		}
	}

	return value, nil
}

func (p *datadogProvider) getMetricNames() ([]string, error) {
	metricNames := []string{}

	fileContents, err := ioutil.ReadFile("/opt/datadog-agent/dev/dist/metrics.yaml")
	if err != nil {
		return nil, fmt.Errorf("error reading metrics file at /opt/datadog-agent/dev/dist/metrics.yaml: %s", err)
	}
	unmarshalledConfig := config{}
	err = yaml.Unmarshal(fileContents, &unmarshalledConfig)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling metrics file: %s", err)
	}

	for _, metric := range unmarshalledConfig.Metrics {
		metricNames = append(metricNames, metric.Name)
	}
	return metricNames, nil
}

func (p *datadogProvider) metricFor(value int64, groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return &custom_metrics.MetricValue{
		DescribedObject: custom_metrics.ObjectReference{
			APIVersion: groupResource.Group + "/" + runtime.APIVersionInternal,
			Kind:       "",
			Name:       name,
			Namespace:  namespace,
		},
		MetricName: metricName,
		Timestamp:  metav1.Time{time.Now()},
		Value:      *resource.NewQuantity(value, resource.DecimalSI),
	}, nil
}

func (p *datadogProvider) metricsFor(totalValue int64, groupResource schema.GroupResource, metricName string, list runtime.Object) (*custom_metrics.MetricValueList, error) {
	if !apimeta.IsListType(list) {
		return nil, fmt.Errorf("returned object was not a list")
	}

	res := make([]custom_metrics.MetricValue, 0)

	err := apimeta.EachListItem(list, func(item runtime.Object) error {
		objMeta := item.(metav1.Object)
		value, err := p.metricFor(0, groupResource, objMeta.GetNamespace(), objMeta.GetName(), metricName)
		if err != nil {
			return err
		}
		res = append(res, *value)

		return nil
	})
	if err != nil {
		return nil, err
	}

	for i := range res {
		res[i].Value = *resource.NewQuantity(totalValue, resource.DecimalSI)
	}

	return &custom_metrics.MetricValueList{
		Items: res,
	}, nil
}

func (p *datadogProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("Not Implemented - RootScopedByName")
}

func (p *datadogProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	// // construct a client to list the names of objects matching the label selector
	// client, err := p.client.ClientForGroupVersionResource(groupResource.WithVersion(""))
	// if err != nil {
	// 	glog.Errorf("unable to construct dynamic client to list matching resource names: %v", err)
	// 	// don't leak implementation details to the user
	// 	return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	// }

	// totalValue, err := p.valueFor(groupResource, metricName, false)
	// if err != nil {
	// 	return nil, err
	// }

	// // we can construct a this APIResource ourself, since the dynamic client only uses Name and Namespaced
	// apiRes := &metav1.APIResource{
	// 	Name:       groupResource.Resource,
	// 	Namespaced: false,
	// }

	// matchingObjectsRaw, err := client.Resource(apiRes, "").
	// 	List(metav1.ListOptions{LabelSelector: selector.String()})
	// if err != nil {
	// 	return nil, err
	// }
	// return p.metricsFor(totalValue, groupResource, metricName, matchingObjectsRaw)
	return nil, fmt.Errorf("Not Implemented - RootScopedBySelector")
}

func (p *datadogProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	value, err := p.getValue(metricName)
	if err != nil {
		seelog.Warn("Could not get metric value, defaulting to 130: ", err)
		value = 130
	}
	return p.metricFor(value, groupResource, namespace, name, metricName)
}

func (p *datadogProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	value, err := p.getValue(metricName)
	if err != nil {
		seelog.Warn("Could not get metric value, defaulting to 130: ", err)
		value = 130
	}

	// construct a client to list the names of objects matching the label selector
	client, err := p.client.ClientForGroupVersionResource(groupResource.WithVersion(""))
	if err != nil {
		seelog.Errorf("unable to construct dynamic client to list matching resource names: %v", err)
		// don't leak implementation details to the user
		return nil, apierr.NewInternalError(fmt.Errorf("unable to list matching resources"))
	}

	//totalValue, err := p.valueFor(groupResource, metricName, true)
	if err != nil {
		return nil, err
	}

	// we can construct a this APIResource ourself, since the dynamic client only uses Name and Namespaced
	apiRes := &metav1.APIResource{
		Name:       groupResource.Resource,
		Namespaced: true,
	}

	matchingObjectsRaw, err := client.Resource(apiRes, namespace).
		List(metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}

	seelog.Warnf("in NamespacedBySelector. goupResour e is %v namespace is %v: , selctor is %v, metricName is %v", groupResource, namespace, selector, metricName)
	return p.metricsFor(value, groupResource, metricName, matchingObjectsRaw)
}

func (p *datadogProvider) ListAllMetrics() []provider.CustomMetricInfo {
	metricNames, err := p.getMetricNames()
	if err != nil {
		seelog.Error("Could not get metric list, defaulting to hardcoded one: ", err)
		return []provider.CustomMetricInfo{
			{
				GroupResource: schema.GroupResource{"","pod"},
				Metric:        "nginx.net.connections",
				Namespaced:    true,
			},
		}
	}

	metricInfos := []provider.CustomMetricInfo{}
	for _, metricName := range metricNames {
		metricInfos = append(metricInfos, provider.CustomMetricInfo{
			GroupResource: schema.GroupResource{Group: "", Resource: "pods"},
			Metric:        metricName,
			Namespaced:    true,
		})
	}
	return metricInfos
}

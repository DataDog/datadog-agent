package apiserver

import (
	"fmt"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	agentconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strings"
)

func QueryDatadogExtra(metricName string, tags map[string]string) (int64, error) {
	client := datadog.NewClient(agentconfig.Datadog.GetString("api_key"), agentconfig.Datadog.GetString("app_key"))
	datadogTags := []string{}
	log.Infof("tags are %#v", tags)
	for key, val := range tags {
		log.Infof("tag evaluated is %#v, and %#v", key, val)
		datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", key, val))
	}
	tagEnd := strings.Join(datadogTags, ",")
	log.Infof("tagend is %#v", tagEnd)

	query := fmt.Sprintf("avg:%s{%s}", metricName, tagEnd)

	seriesSlice, err := client.QueryMetrics(time.Now().Unix()-5*60, time.Now().Unix(), query)

	if err != nil {
		return 0, fmt.Errorf("Error while executing metric query %s: %s", query, err)
	}
	log.Infof("evaluating %s", query)
	log.Infof("collected %s", seriesSlice)

	if len(seriesSlice) < 1 {
		return 0, fmt.Errorf("Returned series slice empty")
	}

	points := seriesSlice[0].Points
	if len(seriesSlice[0].Points) < 1 {
		return 0, fmt.Errorf("No points in series")
	}

	lastValue := int64(points[len(points)-1][1])
	return lastValue, nil
}

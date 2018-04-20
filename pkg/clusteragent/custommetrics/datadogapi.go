package custommetrics

import (
	"fmt"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	agentconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func queryDatadog(metricName string) (int64, error) {
	client := datadog.NewClient(agentconfig.Datadog.GetString("api_key"), agentconfig.Datadog.GetString("app_key"))
	query := fmt.Sprintf("avg:%s{kube_service:nginx}", metricName)
	seriesSlice, err := client.QueryMetrics(time.Now().Unix()-5*60, time.Now().Unix(), query)
	if err != nil {
		return 0, fmt.Errorf("Error while executing metric query %s: %s", query, err)
	}

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

package models

import (
	"github.com/DataDog/datadog-agent/aggregator"
)

type Check interface {
	Check(agg *aggregator.Aggregator)
}

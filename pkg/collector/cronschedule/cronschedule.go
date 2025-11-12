package cronschedule

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/robfig/cron/v3"
)

type CronSchedule struct {
	cronSchedule cron.Schedule
	cronNext     time.Time
}

func NewCronSchedule(cronStr string) (*CronSchedule, error) {
	cronSchedule, err := cron.ParseStandard(cronStr)
	if err != nil {
		return nil, err
	}
	return &CronSchedule{
		cronSchedule: cronSchedule,
	}, nil
}

func (c *CronSchedule) ShouldRun(t time.Time) bool {
	log.Warnf("[CronShouldRun] t %s", t)
	log.Warnf("[CronShouldRun] cronNext %s", c.cronNext)
	if c.cronNext.IsZero() {
		c.cronNext = c.cronSchedule.Next(t)
	}
	if c.cronNext.Before(t) || c.cronNext.Equal(t) {
		log.Warnf("[CronShouldRun] cronNext2 %s", c.cronNext)
		// TODO: Need to skip many scheduled if the backlog is too big?
		c.cronNext = c.cronSchedule.Next(c.cronNext)
		log.Warnf("[CronShouldRun] cronNext3 %s", c.cronNext)
		return true
	}
	return false
}

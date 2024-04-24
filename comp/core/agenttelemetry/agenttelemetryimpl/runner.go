// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// ---------------------------------------------------
//
// This is experimental code and is subject to change.
//
// ---------------------------------------------------

package agenttelemetryimpl

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
)

type runner interface {
	start()
	stop() context.Context
	addJob(j job)
}

type runnerImpl struct {
	taskCron *cron.Cron
}

type runnerSchedule struct {
	schedule Schedule
	firstRun time.Time
	iterated uint
}

// Agent telemetry runner currently able to "prosecute" only schedulers expressed
// in the following format
//
//   "schedule": {
// 	     "period": xxxx,
// 	     "iterations": xxxx,
// 	     "start_after": xxxx
//   }
//
//   ... where ...
//      "period" is the period (time between one job to another) in seconds,
//      "iterations" is the number of time the job should be executed (0 means forever),
//	    "start_after" is the time to wait before the first run in seconds (effectively a delay after initialization)
//

// Using well-know robfig/cron package to schedule jobs without complex tracking of multiple schedules
// and to avoid reinventing the wheel. The package is well tested and maintained. Its source tracks multiuple
// job scheduling in simple and elegant way.

func (rs *runnerSchedule) Next(now time.Time) time.Time {
	// Is it first time?
	if rs.firstRun.IsZero() {
		rs.firstRun = now.Add(time.Second * time.Duration(rs.schedule.StartAfter))
		return rs.firstRun
	}

	// By this point, we have run or will be run shortly
	rs.iterated++

	// Is it time to stop?
	if rs.schedule.Iterations > 0 && rs.iterated >= rs.schedule.Iterations {
		// return zero time to stop the schedule
		return time.Time{}

		// In future we may want to remove the schedule from the cron it cannot be done from this function,
		// by the way (I have tried) because cron.Remove(taskId) requires Lock which is already taken
		// the this function runs. It has to be done externally.
	}

	// Return next run time
	return now.Add(time.Second * time.Duration(rs.schedule.Period))
}

// newRunner registers callback.
func newRunnerImpl() runner {
	return &runnerImpl{}
}

func (r *runnerImpl) start() {
	r.taskCron = cron.New()
	r.taskCron.Start()
}

func (r *runnerImpl) stop() context.Context {
	return r.taskCron.Stop()
}

func (r *runnerImpl) addJob(j job) {
	schedule := &runnerSchedule{
		schedule: j.schedule,
	}
	r.taskCron.Schedule(schedule, j)
}

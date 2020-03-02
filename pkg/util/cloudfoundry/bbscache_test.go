// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package cloudfoundry

import (
	"context"
	"os"
	"testing"
	"time"

	"code.cloudfoundry.org/bbs/events"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/lager"
	"github.com/stretchr/testify/assert"
)

type testBBSClient struct {
}

func (t testBBSClient) ActualLRPs(lager.Logger, models.ActualLRPFilter) ([]*models.ActualLRP, error) {
	return []*models.ActualLRP{&BBSModelA1, &BBSModelA2}, nil
}

func (t testBBSClient) DesiredLRPs(lager.Logger, models.DesiredLRPFilter) ([]*models.DesiredLRP, error) {
	return []*models.DesiredLRP{&BBSModelD1}, nil
}

var c *BBSCache

func TestMain(m *testing.M) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ = ConfigureGlobalBBSCache(ctx, "url", "", "", "", time.Second, &testBBSClient{})
	for range []int{0, 1} {
		if c.GetPollSuccesses() == 0 {
			time.Sleep(time.Second)
		}
	}
	code := m.Run()
	os.Exit(code)
}

func TestBBSCachePolling(t *testing.T) {
	assert.NotZero(t, c.GetPollAttempts())
	assert.NotZero(t, c.GetPollSuccesses())
}

func TestBBSCache_GetDesiredLRPFor(t *testing.T) {
	assert.EqualValues(t, ExpectedD1, c.GetDesiredLRPFor("012345678901234567890123456789012345"))
}

func TestBBSCache_GetActualLRPFor(t *testing.T) {
	assert.EqualValues(t, ExpectedA1, c.GetActualLRPFor("0123456789012345678"))
	assert.EqualValues(t, ExpectedA2, c.GetActualLRPFor("0123456789012345679"))
}

func TestBBSCache_ExtractTags(t *testing.T) {
	expectedTags := map[string][]string{
		"0123456789012345678": {
			"container_name:name_of_the_app_4",
			"app_name:name_of_the_app",
			"app_guid:012345678901234567890123456789012345",
			"app_instance_index:4",
			"app_instance_guid:0123456789012345678",
		},
	}
	assert.Equal(t, expectedTags, c.ExtractTags("cell123"))
	expectedTags = map[string][]string{
		"0123456789012345679": {
			"container_name:name_of_the_app_3",
			"app_name:name_of_the_app",
			"app_guid:012345678901234567890123456789012345",
			"app_instance_index:3",
			"app_instance_guid:0123456789012345679",
		},
	}
	assert.Equal(t, expectedTags, c.ExtractTags("cell1234"))
}

func TestBBSCache_GetActualLRPsFor(t *testing.T) {
	assert.EqualValues(t, []ActualLRP{ExpectedA1, ExpectedA2}, c.GetActualLRPsFor("012345678901234567890123456789012345"))
}

func TestBBSCache_GetAllLRPs(t *testing.T) {
	a, d := c.GetAllLRPs()
	assert.EqualValues(t, map[string]DesiredLRP{ExpectedD1.AppGUID: ExpectedD1}, d)
	assert.EqualValues(t, map[string][]ActualLRP{"012345678901234567890123456789012345": {ExpectedA1, ExpectedA2}}, a)
}

// These methods ensure we implement the bbs.Client API, but are in fact unused by our functionality
func (t testBBSClient) DesireTask(logger lager.Logger, guid, domain string, def *models.TaskDefinition) error {
	panic("implement me")
}

func (t testBBSClient) Tasks(logger lager.Logger) ([]*models.Task, error) {
	panic("implement me")
}

func (t testBBSClient) TasksWithFilter(logger lager.Logger, filter models.TaskFilter) ([]*models.Task, error) {
	panic("implement me")
}

func (t testBBSClient) TasksByDomain(logger lager.Logger, domain string) ([]*models.Task, error) {
	panic("implement me")
}

func (t testBBSClient) TasksByCellID(logger lager.Logger, cellID string) ([]*models.Task, error) {
	panic("implement me")
}

func (t testBBSClient) TaskByGuid(logger lager.Logger, guid string) (*models.Task, error) {
	panic("implement me")
}

func (t testBBSClient) CancelTask(logger lager.Logger, taskGUID string) error {
	panic("implement me")
}

func (t testBBSClient) ResolvingTask(logger lager.Logger, taskGUID string) error {
	panic("implement me")
}

func (t testBBSClient) DeleteTask(logger lager.Logger, taskGUID string) error {
	panic("implement me")
}

func (t testBBSClient) Domains(logger lager.Logger) ([]string, error) {
	panic("implement me")
}

func (t testBBSClient) UpsertDomain(logger lager.Logger, domain string, ttl time.Duration) error {
	panic("implement me")
}

func (t testBBSClient) ActualLRPGroups(lager.Logger, models.ActualLRPFilter) ([]*models.ActualLRPGroup, error) {
	panic("implement me")
}

func (t testBBSClient) ActualLRPGroupsByProcessGuid(logger lager.Logger, processGUID string) ([]*models.ActualLRPGroup, error) {
	panic("implement me")
}

func (t testBBSClient) ActualLRPGroupByProcessGuidAndIndex(logger lager.Logger, processGUID string, index int) (*models.ActualLRPGroup, error) {
	panic("implement me")
}

func (t testBBSClient) RetireActualLRP(logger lager.Logger, key *models.ActualLRPKey) error {
	panic("implement me")
}

func (t testBBSClient) DesiredLRPByProcessGuid(logger lager.Logger, processGUID string) (*models.DesiredLRP, error) {
	panic("implement me")
}

func (t testBBSClient) DesiredLRPSchedulingInfos(lager.Logger, models.DesiredLRPFilter) ([]*models.DesiredLRPSchedulingInfo, error) {
	panic("implement me")
}

func (t testBBSClient) DesireLRP(lager.Logger, *models.DesiredLRP) error {
	panic("implement me")
}

func (t testBBSClient) UpdateDesiredLRP(logger lager.Logger, processGUID string, update *models.DesiredLRPUpdate) error {
	panic("implement me")
}

func (t testBBSClient) RemoveDesiredLRP(logger lager.Logger, processGUID string) error {
	panic("implement me")
}

func (t testBBSClient) SubscribeToEvents(logger lager.Logger) (events.EventSource, error) {
	panic("implement me")
}

func (t testBBSClient) SubscribeToInstanceEvents(logger lager.Logger) (events.EventSource, error) {
	panic("implement me")
}

func (t testBBSClient) SubscribeToTaskEvents(logger lager.Logger) (events.EventSource, error) {
	panic("implement me")
}

func (t testBBSClient) SubscribeToEventsByCellID(logger lager.Logger, cellID string) (events.EventSource, error) {
	panic("implement me")
}

func (t testBBSClient) SubscribeToInstanceEventsByCellID(logger lager.Logger, cellID string) (events.EventSource, error) {
	panic("implement me")
}

func (t testBBSClient) Ping(logger lager.Logger) bool {
	panic("implement me")
}

func (t testBBSClient) Cells(logger lager.Logger) ([]*models.CellPresence, error) {
	panic("implement me")
}

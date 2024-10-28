// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && !windows

package cloudfoundry

import (
	"testing"
	"time"

	"code.cloudfoundry.org/bbs/events"
	"code.cloudfoundry.org/bbs/models"
	"code.cloudfoundry.org/lager"
	"github.com/stretchr/testify/assert"
)

func (t testBBSClient) ActualLRPs(lager.Logger, models.ActualLRPFilter) ([]*models.ActualLRP, error) {
	return []*models.ActualLRP{&BBSModelA1, &BBSModelA2}, nil
}

func (t testBBSClient) DesiredLRPs(lager.Logger, models.DesiredLRPFilter) ([]*models.DesiredLRP, error) {
	return []*models.DesiredLRP{&BBSModelD1}, nil
}

func TestBBSCachePolling(t *testing.T) {
	assert.NotZero(t, bc.LastUpdated())
}

func TestBBSCache_GetDesiredLRPFor(t *testing.T) {
	dlrp, _ := bc.GetDesiredLRPFor("0123456789012345678901234567890123456789")
	assert.EqualValues(t, ExpectedD1, dlrp)
}

func TestBBSCache_GetActualLRPsForCell(t *testing.T) {
	alrp, _ := bc.GetActualLRPsForCell("cell123")
	assert.EqualValues(t, []*ActualLRP{&ExpectedA1}, alrp)
	alrp, _ = bc.GetActualLRPsForCell("cell1234")
	assert.EqualValues(t, []*ActualLRP{&ExpectedA2}, alrp)
}

func TestBBSCache_GetTagsForNode(t *testing.T) {
	expectedTags := map[string][]string{
		"0123456789012345678": {
			"container_name:name_of_app_cc_4",
			"app_instance_index:4",
			"app_instance_guid:0123456789012345678",
			"app_guid:random_app_guid",
			"app_id:random_app_guid",
			"app_name:name_of_app_cc",
			"env:test-env",
			"org_id:org_guid_1",
			"org_name:org_name_1",
			"segment_id:isolation_segment_guid_1",
			"segment_name:isolation_segment_name_1",
			"service:test-service",
			"sidecar_count:1",
			"sidecar_present:true",
			"space_id:space_guid_1",
			"space_name:space_name_1",
		},
	}
	tags, err := bc.GetTagsForNode("cell123")
	assert.Nil(t, err)
	assert.Equal(t, expectedTags, tags)
	expectedTags = map[string][]string{
		"0123456789012345679": {
			"container_name:name_of_app_cc_3",
			"app_instance_index:3",
			"app_instance_guid:0123456789012345679",
			"app_guid:random_app_guid",
			"app_id:random_app_guid",
			"app_name:name_of_app_cc",
			"env:test-env",
			"org_id:org_guid_1",
			"org_name:org_name_1",
			"segment_id:isolation_segment_guid_1",
			"segment_name:isolation_segment_name_1",
			"service:test-service",
			"sidecar_count:1",
			"sidecar_present:true",
			"space_id:space_guid_1",
			"space_name:space_name_1",
		},
	}
	tags, err = bc.GetTagsForNode("cell1234")
	assert.Nil(t, err)
	assert.Equal(t, expectedTags, tags)
}

func TestBBSCache_GetActualLRPsForProcessGUID(t *testing.T) {
	alrps, _ := bc.GetActualLRPsForProcessGUID("0123456789012345678901234567890123456789")
	assert.EqualValues(t, []*ActualLRP{&ExpectedA1, &ExpectedA2}, alrps)
}

func TestBBSCache_GetAllLRPs(t *testing.T) {
	a, d := bc.GetAllLRPs()
	assert.EqualValues(t, map[string]*DesiredLRP{ExpectedD1.ProcessGUID: &ExpectedD1}, d)
	assert.EqualValues(t, map[string][]*ActualLRP{ExpectedD1.ProcessGUID: {&ExpectedA1, &ExpectedA2}}, a)
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

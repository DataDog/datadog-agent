// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build ec2

package listeners

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/aws"
	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/rds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DBMRdsListener implements database-monitoring Rds discovery
type DBMRdsListener struct {
	sync.RWMutex
	newService   chan<- Service
	delService   chan<- Service
	stop         chan bool
	services     map[string]Service
	config       rds.Config
	awsRdsClient aws.RdsClient
	// ticks is used primarily for testing purposes so
	// the frequency the discovers loop iterates can be controlled
	ticks  <-chan time.Time
	ticker *time.Ticker
}

var _ Service = &DBMRdsService{}

// DBMRdsService implements and store results from the Service interface for the DBMRdsListener
type DBMRdsService struct {
	adIdentifier string
	entityID     string
	checkName    string
	region       string
	instance     *aws.Instance
}

// NewDBMRdsListener returns a new DBMRdsListener
func NewDBMRdsListener(ServiceListernerDeps) (ServiceListener, error) {
	config, err := rds.NewRdsAutodiscoveryConfig()
	if err != nil {
		return nil, err
	}
	client, region, err := aws.NewRdsClient(config.Region)
	if err != nil {
		return nil, err
	}
	config.Region = region
	return newDBMRdsListener(config, client, nil), nil
}

func newDBMRdsListener(config rds.Config, awsClient aws.RdsClient, ticks <-chan time.Time) ServiceListener {
	l := &DBMRdsListener{
		config:       config,
		services:     make(map[string]Service),
		stop:         make(chan bool),
		awsRdsClient: awsClient,
		ticks:        ticks,
	}
	if l.ticks == nil {
		l.ticker = time.NewTicker(time.Duration(l.config.DiscoveryInterval) * time.Second)
		l.ticks = l.ticker.C
	}
	return l
}

// Listen listens for new and deleted rds endpoints
func (l *DBMRdsListener) Listen(newSvc, delSvc chan<- Service) {
	l.newService = newSvc
	l.delService = delSvc
	go l.run()
}

// Stop stops the listener
func (l *DBMRdsListener) Stop() {
	l.stop <- true
	if l.ticker != nil {
		l.ticker.Stop()
	}
}

// run is the main loop for the rds listener discovery
func (l *DBMRdsListener) run() {
	for {
		l.discoverRdsInstances()
		select {
		case <-l.stop:
			return
		case <-l.ticks:
		}
	}
}

// discoverRdsInstances discovers rds instances according to the configuration
func (l *DBMRdsListener) discoverRdsInstances() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(l.config.QueryTimeout)*time.Second)
	defer cancel()
	instances, err := l.awsRdsClient.GetRdsInstancesFromTags(ctx, l.config.Tags, l.config.DbmTag)
	if err != nil {
		_ = log.Error(err)
		return
	}
	if len(instances) == 0 {
		log.Debugf("no rds instances found with provided tags %v", l.config.Tags)
		return
	}
	log.Debugf("found %d rds instances with provided tags %v", len(instances), l.config.Tags)
	discoveredServices := make(map[string]struct{})
	for _, instance := range instances {
		log.Debugf("found rds instance %v", instance)
		entityID := instance.Digest(engineToIntegrationType[instance.Engine], instance.ID)
		discoveredServices[entityID] = struct{}{}
		l.createService(entityID, instance)
	}

	deletedServices := findDeletedServices(l.services, discoveredServices)
	l.deleteServices(deletedServices)
}

func (l *DBMRdsListener) createService(entityID string, instance aws.Instance) {
	if _, present := l.services[entityID]; present {
		return
	}
	svc := &DBMRdsService{
		adIdentifier: engineToRdsADIdentifier[instance.Engine],
		entityID:     entityID,
		checkName:    engineToIntegrationType[instance.Engine],
		instance:     &instance,
		region:       l.config.Region,
	}
	l.services[entityID] = svc
	l.newService <- svc
}

func (l *DBMRdsListener) deleteServices(entityIDs []string) {
	for _, entityID := range entityIDs {
		if svc, present := l.services[entityID]; present {
			l.delService <- svc
			delete(l.services, entityID)
		}
	}
}

// Equal returns whether the two DBMRdsService are equal
func (d *DBMRdsService) Equal(o Service) bool {
	d2, ok := o.(*DBMRdsService)
	if !ok {
		return false
	}

	return d.adIdentifier == d2.adIdentifier &&
		d.entityID == d2.entityID &&
		d.checkName == d2.checkName &&
		d.region == d2.region &&
		reflect.DeepEqual(d.instance, d2.instance)
}

// GetServiceID returns the unique entity name linked to that service
func (d *DBMRdsService) GetServiceID() string {
	return d.entityID
}

// GetTaggerEntity returns the tagger entity
func (d *DBMRdsService) GetTaggerEntity() string {
	return d.entityID
}

// GetADIdentifiers return the single AD identifier for a static config service
func (d *DBMRdsService) GetADIdentifiers() []string {
	return []string{d.adIdentifier}
}

// GetHosts returns the host for the rds endpoint
func (d *DBMRdsService) GetHosts() (map[string]string, error) {
	return map[string]string{"": d.instance.Endpoint}, nil
}

// GetPorts returns the port for the rds endpoint
func (d *DBMRdsService) GetPorts() ([]ContainerPort, error) {
	port := int(d.instance.Port)
	return []ContainerPort{{port, fmt.Sprintf("p%d", port)}}, nil
}

// GetTags returns the list of container tags - currently always empty
func (d *DBMRdsService) GetTags() ([]string, error) {
	return []string{}, nil
}

// GetTagsWithCardinality returns the tags with given cardinality. Not supported in DBMRdsService
func (d *DBMRdsService) GetTagsWithCardinality(_ string) ([]string, error) {
	return d.GetTags()
}

// GetPid returns nil and an error because pids are currently not supported
func (d *DBMRdsService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nothing - not supported
func (d *DBMRdsService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// IsReady returns true on DBMRdsService
func (d *DBMRdsService) IsReady() bool {
	return true
}

// GetCheckNames returns the check name for the service.
func (d *DBMRdsService) GetCheckNames(context.Context) []string {
	return []string{d.checkName}
}

// HasFilter returns false on DBMRdsService
func (d *DBMRdsService) HasFilter(filter.Scope) bool {
	return false
}

// GetExtraConfig parses the template variables with the extra_ prefix and returns the value
func (d *DBMRdsService) GetExtraConfig(key string) (string, error) {
	switch key {
	case "dbm":
		return strconv.FormatBool(d.instance.DbmEnabled), nil
	case "region":
		return d.region, nil
	case "managed_authentication_enabled":
		return strconv.FormatBool(d.instance.IamEnabled), nil
	case "dbinstanceidentifier":
		return d.instance.ID, nil
	case "dbclusteridentifier":
		return d.instance.ClusterID, nil
	case "dbname":
		return d.instance.DbName, nil
	}

	return "", ErrNotSupported
}

// FilterTemplates does nothing.
func (d *DBMRdsService) FilterTemplates(map[string]integration.Config) {
}

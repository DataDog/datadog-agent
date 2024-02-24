// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package listeners

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/databasemonitoring/aws"
	dbmconfig "github.com/DataDog/datadog-agent/pkg/databasemonitoring/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strconv"
	"sync"
	"time"
)

const dbmAdIdentifier = "_dbm_aws_aurora"

func init() {
	Register(dbmAdIdentifier, NewDBMAuroraListener)
}

// DBMAuroraListener implements database-monitoring aurora discovery
type DBMAuroraListener struct {
	sync.RWMutex
	newService   chan<- Service
	delService   chan<- Service
	stop         chan bool
	services     map[string]Service
	config       dbmconfig.AuroraConfig
	awsRdsClient aws.RDSClient
	// ticks is used primarily for testing purposes so
	// the frequency the discovers loop iterates can be controlled
	ticks  <-chan time.Time
	ticker *time.Ticker
}

var _ Service = &DBMAuroraService{}

// DBMAuroraService implements and store results from the Service interface for the DBMAuroraListener
type DBMAuroraService struct {
	adIdentifier string
	entityID     string
	checkName    string
	clusterID    string
	region       string
	instance     *aws.Instance
}

// NewDBMAuroraListener returns a new DBMAuroraListener
func NewDBMAuroraListener(Config) (ServiceListener, error) {
	config, err := dbmconfig.NewAuroraAutodiscoveryConfig()
	if err != nil {
		return nil, err
	}
	client, err := aws.NewRDSClient(config.Region, config.RoleArn)
	if err != nil {
		return nil, err
	}
	return newDBMAuroraListener(config, client, nil), nil
}

func newDBMAuroraListener(config dbmconfig.AuroraConfig, awsClient aws.RDSClient, ticks <-chan time.Time) ServiceListener {
	l := &DBMAuroraListener{
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

// Listen listens for new and deleted aurora endpoints
func (l *DBMAuroraListener) Listen(newSvc, delSvc chan<- Service) {
	l.newService = newSvc
	l.delService = delSvc
	go l.run()
}

// Stop stops the listener
func (l *DBMAuroraListener) Stop() {
	l.stop <- true
	if l.ticker != nil {
		l.ticker.Stop()
	}
}

// run is the main loop for the aurora listener discovery
func (l *DBMAuroraListener) run() {
	for {
		l.discoverAuroraClusters()
		select {
		case <-l.stop:
			return
		case <-l.ticks:
		}
	}
}

// discoverAuroraClusters discovers aurora clusters according to the configuration
func (l *DBMAuroraListener) discoverAuroraClusters() {
	for _, cluster := range l.config.Clusters {
		ids := make([]string, 0)
		ids = append(ids, cluster.ClusterIds...)
		auroraCluster, err := func() (map[string]*aws.AuroraCluster, error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(l.config.QueryTimeout)*time.Second)
			defer cancel()
			return l.awsRdsClient.GetAuroraClusterEndpoints(ctx, ids)
		}()
		if err != nil {
			_ = log.Error(err)
			continue
		}
		discoveredServices := make(map[string]struct{})
		for id, c := range auroraCluster {
			for _, instance := range c.Instances {
				if instance == nil {
					_ = log.Warnf("received malformed instance response for cluster %s, skipping", id)
					continue
				}
				entityID := instance.Digest(string(cluster.Type), l.config.Region, id)
				discoveredServices[entityID] = struct{}{}
				l.createService(entityID, string(cluster.Type), id, l.config.Region, instance)
			}
		}
		// TODO: should we wait a certain number of run iterations before we remove instances?
		deletedServices := findDeletedServices(l.services, discoveredServices)
		l.deleteServices(deletedServices)
	}
}

func (l *DBMAuroraListener) createService(entityID, checkName, clusterID, region string, instance *aws.Instance) {
	l.Lock()
	defer l.Unlock()
	if _, present := l.services[entityID]; present {
		return
	}
	svc := &DBMAuroraService{
		adIdentifier: dbmAdIdentifier,
		entityID:     entityID,
		checkName:    checkName,
		instance:     instance,
		clusterID:    clusterID,
		region:       region,
	}
	l.services[entityID] = svc
	l.newService <- svc
}

func (l *DBMAuroraListener) deleteServices(entityIDs []string) {
	l.Lock()
	defer l.Unlock()
	for _, entityID := range entityIDs {
		if svc, present := l.services[entityID]; present {
			l.delService <- svc
			delete(l.services, entityID)
		}
	}
}

func findDeletedServices(currServices map[string]Service, discoveredServices map[string]struct{}) []string {
	deletedServices := make([]string, 0)
	for svc, _ := range currServices {
		if _, exists := discoveredServices[svc]; !exists {
			deletedServices = append(deletedServices, svc)
		}
	}

	return deletedServices
}

// GetServiceID returns the unique entity name linked to that service
func (d *DBMAuroraService) GetServiceID() string {
	return d.entityID
}

// GetTaggerEntity returns the tagger entity
func (d *DBMAuroraService) GetTaggerEntity() string {
	return d.entityID
}

// GetADIdentifiers return the single AD identifier for a static config service
func (d *DBMAuroraService) GetADIdentifiers(context.Context) ([]string, error) {
	return []string{d.adIdentifier}, nil
}

// GetHosts returns the host for the aurora endpoint
func (d *DBMAuroraService) GetHosts(context.Context) (map[string]string, error) {
	return map[string]string{"": d.instance.Endpoint}, nil
}

// GetPorts returns the port for the aurora endpoint
func (d *DBMAuroraService) GetPorts(context.Context) ([]ContainerPort, error) {
	port := int(d.instance.Port)
	return []ContainerPort{{port, fmt.Sprintf("p%d", port)}}, nil
}

// GetTags returns the list of container tags - currently always empty
func (d *DBMAuroraService) GetTags() ([]string, error) {
	return []string{}, nil
}

// GetPid returns nil and an error because pids are currently not supported
func (d *DBMAuroraService) GetPid(context.Context) (int, error) {
	return -1, ErrNotSupported
}

// GetHostname returns nothing - not supported
func (d *DBMAuroraService) GetHostname(context.Context) (string, error) {
	return "", ErrNotSupported
}

// IsReady returns true on DBMAuroraService
func (d *DBMAuroraService) IsReady(context.Context) bool {
	return true
}

// GetCheckNames returns nil
func (d *DBMAuroraService) GetCheckNames(context.Context) []string {
	return []string{d.checkName}
}

// HasFilter returns false on DBMAuroraService
func (d *DBMAuroraService) HasFilter(containers.FilterType) bool {
	return false
}

// GetExtraConfig parses the template variables with the extra_ prefix and returns the value
func (d *DBMAuroraService) GetExtraConfig(key string) (string, error) {
	switch key {
	case "region":
		return d.region, nil
	case "managed_authentication_enabled":
		return strconv.FormatBool(d.instance.IamEnabled), nil
	case "dbclusteridentifier":
		return d.clusterID, nil
	}
	return "", ErrNotSupported
}

// FilterTemplates does nothing.
//
//nolint:revive
func (d *DBMAuroraService) FilterTemplates(m map[string]integration.Config) {
}

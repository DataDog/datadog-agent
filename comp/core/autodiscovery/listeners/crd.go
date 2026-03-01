// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package listeners

import (
	"errors"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type CRDListener struct {
	workloadmetaListener
}

// todo: to enable this listener currently set env variable DD_EXTERNAL_LISTENERS="... crd"

// NewCRDListerner returns a new CRDListener
func NewCRDListerner(options ServiceListernerDeps) (ServiceListener, error) {
	l := &CRDListener{}

	wmetaFilter := workloadmeta.NewFilterBuilder().
		SetSource(workloadmeta.SourceKubeAPIServer).
		AddKind(workloadmeta.KindCRD).
		Build()

	wmetaInstance, ok := options.Wmeta.Get()
	if !ok {
		return nil, errors.New("workloadmeta is not initialized")
	}

	var err error
	l.workloadmetaListener, err = newWorkloadmetaListener(
		"ad-crdlistener",
		wmetaFilter,
		l.processFn,
		wmetaInstance,
		options.Telemetry,
	)

	if err != nil {
		return nil, errors.Join(errors.New("failed to initialize workload-meta listern"), err)
	}

	return l, nil
}

// processCRD creates a new Service for each unique CRD from workload-meta
func (l *CRDListener) processFn(e workloadmeta.Entity) {
	crdEntity, ok := e.(*workloadmeta.CRD)
	if !ok {
		log.Errorf("received wrong entity type, expected %T, received %T", &workloadmeta.CRD{}, e)

		return
	}

	adIdentifiers := []string{strings.ToLower(crdEntity.BuildGVK())}

	log.Infof("found new crd: %s - check for known integrations matching ADIdentifiers %s", crdEntity.ID, adIdentifiers)

	s := &CRDService{
		entityID:      crdEntity.ID,
		adIdentifiers: adIdentifiers,
	}

	l.AddService(
		buildSvcID(crdEntity.EntityID),
		s,
		"",
	)
}

type CRDService struct {
	entityID      string
	adIdentifiers []string
}

func (s *CRDService) Equal(o Service) bool {
	other, ok := o.(*CRDService)
	if !ok {
		return false
	}

	return s.entityID == other.entityID &&
		slices.Compare(s.adIdentifiers, other.adIdentifiers) == 0
}

func (s *CRDService) GetServiceID() string {
	return s.entityID
}

func (s *CRDService) GetADIdentifiers() []string {
	return s.adIdentifiers
}

func (s *CRDService) GetHosts() (map[string]string, error) {
	return nil, ErrNotSupported
}

func (s *CRDService) GetPorts() ([]workloadmeta.ContainerPort, error) {
	return nil, ErrNotSupported
}

func (s *CRDService) GetTags() ([]string, error) {
	return []string{}, nil
}

func (s *CRDService) GetTagsWithCardinality(_ string) ([]string, error) {
	return []string{}, nil
}

func (s *CRDService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

func (s *CRDService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

func (s *CRDService) IsReady() bool {
	return true
}

func (s *CRDService) HasFilter(_ workloadfilter.Scope) bool {
	return false
}

func (s *CRDService) GetExtraConfig(_ string) (string, error) {
	return "", ErrNotSupported
}

func (s *CRDService) GetImageName() string {
	return ""
}

// FilterTemplates filters the templates which will be resolved against
// this service, in a map keyed by template digest.
//
// This method is called every time the configs for the service change,
// with the full set of templates matching this service.  It must not rely
// on any non-static information except the given configs, and it must not
// modify the configs in the map.
func (s *CRDService) FilterTemplates(_ map[string]integration.Config) {
}

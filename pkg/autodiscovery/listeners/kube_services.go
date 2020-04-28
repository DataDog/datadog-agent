// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package listeners

import (
	"fmt"
	"sort"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	infov1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kubeServiceAnnotationFormat = "ad.datadoghq.com/service.instances"
)

// KubeServiceListener listens to kubernetes service creation
type KubeServiceListener struct {
	informer   infov1.ServiceInformer
	services   map[types.UID]Service
	newService chan<- Service
	delService chan<- Service
	m          sync.RWMutex
}

// KubeServiceService represents a Kubernetes Service
type KubeServiceService struct {
	entity       string
	tags         []string
	hosts        map[string]string
	ports        []ContainerPort
	creationTime integration.CreationTime
}

// Make sure KubeServiceService implements the Service interface
var _ Service = &KubeServiceService{}

func init() {
	Register("kube_services", NewKubeServiceListener)
}

func NewKubeServiceListener() (ServiceListener, error) {
	ac, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("cannot connect to apiserver: %s", err)
	}

	servicesInformer := ac.InformerFactory.Core().V1().Services()
	if servicesInformer == nil {
		return nil, fmt.Errorf("cannot get service informer: %s", err)
	}

	return &KubeServiceListener{
		services: make(map[types.UID]Service),
		informer: servicesInformer,
	}, nil
}

func (l *KubeServiceListener) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	// setup the I/O channels
	l.newService = newSvc
	l.delService = delSvc

	l.informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    l.added,
		UpdateFunc: l.updated,
		DeleteFunc: l.deleted,
	})

	// Initial fill
	services, err := l.informer.Lister().List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes services: %s", err)
	}
	for _, s := range services {
		l.createService(s, true)
	}
}

// Stop is a stub
func (l *KubeServiceListener) Stop() {
	// We cannot deregister from the informer
}

func (l *KubeServiceListener) added(obj interface{}) {
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", obj)
		return
	}
	l.createService(castedObj, false)
}

func (l *KubeServiceListener) deleted(obj interface{}) {
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", obj)
		return
	}
	l.removeService(castedObj)
}

func (l *KubeServiceListener) updated(old, obj interface{}) {
	// Cast the updated object or return on failure
	castedObj, ok := obj.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", obj)
		return
	}
	// Cast the old object, consider it an add on cast failure
	castedOld, ok := old.(*v1.Service)
	if !ok {
		log.Errorf("Expected a Service type, got: %v", old)
		l.createService(castedObj, false)
		return
	}
	if servicesDiffer(castedObj, castedOld) {
		l.removeService(castedObj)
		l.createService(castedObj, false)
	}
}

// servicesDiffer compares two services to only go forward
// when relevant fields are changed. This logic must be
// updated if more fields are used.
func servicesDiffer(first, second *v1.Service) bool {
	// Quick exit if resversion did not change
	if first.ResourceVersion == second.ResourceVersion {
		return false
	}
	// AD annotations - check templates
	if isServiceAnnotated(first, kubeServiceAnnotationFormat) != isServiceAnnotated(second, kubeServiceAnnotationFormat) {
		return true
	}
	// AD labels - standard tags
	if standardTagsDigest(first.GetLabels()) != standardTagsDigest(second.GetLabels()) {
		return true
	}
	// Cluster IP
	if first.Spec.ClusterIP != second.Spec.ClusterIP {
		return true
	}
	// Ports
	if len(first.Spec.Ports) != len(second.Spec.Ports) {
		return true
	}
	for i := range first.Spec.Ports {
		if first.Spec.Ports[i].Name != second.Spec.Ports[i].Name {
			return true
		}
		if first.Spec.Ports[i].Port != second.Spec.Ports[i].Port {
			return true
		}
	}
	// No relevant change
	return false
}

func (l *KubeServiceListener) createService(ksvc *v1.Service, firstRun bool) {
	if ksvc == nil {
		return
	}
	if !isServiceAnnotated(ksvc, kubeServiceAnnotationFormat) {
		// Ignore services with no AD annotation
		return
	}

	svc := processService(ksvc, firstRun)

	l.m.Lock()
	l.services[ksvc.UID] = svc
	l.m.Unlock()

	l.newService <- svc
}

func processService(ksvc *v1.Service, firstRun bool) *KubeServiceService {
	svc := &KubeServiceService{
		entity:       apiserver.EntityForService(ksvc),
		creationTime: integration.After,
	}
	if firstRun {
		svc.creationTime = integration.Before
	}

	// Service tags
	svc.tags = []string{
		fmt.Sprintf("kube_service:%s", ksvc.Name),
		fmt.Sprintf("kube_namespace:%s", ksvc.Namespace),
	}

	// Standard tags from the service's labels
	svc.tags = append(svc.tags, getStandardTags(ksvc.GetLabels())...)

	// Hosts, only use internal ClusterIP for now
	svc.hosts = map[string]string{"cluster": ksvc.Spec.ClusterIP}

	// Ports
	var ports []ContainerPort
	for _, port := range ksvc.Spec.Ports {
		ports = append(ports, ContainerPort{int(port.Port), port.Name})
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})
	svc.ports = ports
	if len(svc.ports) == 0 {
		// Port might not be specified in pod spec
		log.Debugf("No ports found for service %s", ksvc.Name)
	}

	return svc
}

func (l *KubeServiceListener) removeService(ksvc *v1.Service) {
	if ksvc == nil {
		return
	}
	l.m.RLock()
	svc, ok := l.services[ksvc.UID]
	l.m.RUnlock()

	if ok {
		l.m.Lock()
		delete(l.services, ksvc.UID)
		l.m.Unlock()

		l.delService <- svc
	} else {
		log.Debugf("Entity %s not found, not removing", ksvc.UID)
	}
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeServiceService) GetEntity() string {
	return s.entity
}

// GetEntity returns the unique entity name linked to that service
func (s *KubeServiceService) GetTaggerEntity() string {
	return s.entity
}

// GetADIdentifiers returns the service AD identifiers
func (s *KubeServiceService) GetADIdentifiers() ([]string, error) {
	// Only the entity for now, to match on annotation
	return []string{s.entity}, nil
}

// GetHosts returns the pod hosts
func (s *KubeServiceService) GetHosts() (map[string]string, error) {
	return s.hosts, nil
}

// GetPid is not supported for PodContainerService
func (s *KubeServiceService) GetPid() (int, error) {
	return -1, ErrNotSupported
}

// GetPorts returns the container's ports
func (s *KubeServiceService) GetPorts() ([]ContainerPort, error) {
	return s.ports, nil
}

// GetTags retrieves tags
func (s *KubeServiceService) GetTags() ([]string, error) {
	return s.tags, nil
}

// GetHostname returns nil and an error because port is not supported in Kubelet
func (s *KubeServiceService) GetHostname() (string, error) {
	return "", ErrNotSupported
}

// GetCreationTime returns the creation time of the service compare to the agent start.
func (s *KubeServiceService) GetCreationTime() integration.CreationTime {
	return s.creationTime
}

// IsReady returns if the service is ready
func (s *KubeServiceService) IsReady() bool {
	return true
}

// GetCheckNames returns slice of check names defined in kubernetes annotations or docker labels
// KubeServiceService doesn't implement this method
func (s *KubeServiceService) GetCheckNames() []string {
	return nil
}

// HasFilter always return false
// KubeServiceService doesn't implement this method
func (s *KubeServiceService) HasFilter(filter containers.FilterType) bool {
	return false
}

// GetExtraConfig isn't supported
func (s *KubeServiceService) GetExtraConfig(key string) (string, error) {
	return "", ErrNotSupported
}

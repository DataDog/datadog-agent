// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// OrchestratorPayload is a payload type for the orchestrator check
type OrchestratorPayload struct {
	Type          agentmodel.MessageType
	UID           string
	Name          string
	Tags          []string
	CollectedTime time.Time

	Pod                                  *agentmodel.Pod
	PodParentCollector                   *agentmodel.CollectorPod
	ReplicaSet                           *agentmodel.ReplicaSet
	ReplicaSetParentCollector            *agentmodel.CollectorReplicaSet
	Deployment                           *agentmodel.Deployment
	DeploymentParentCollector            *agentmodel.CollectorDeployment
	Service                              *agentmodel.Service
	ServiceParentCollector               *agentmodel.CollectorService
	Node                                 *agentmodel.Node
	NodeParentCollector                  *agentmodel.CollectorNode
	Cluster                              *agentmodel.Cluster
	ClusterParentCollector               *agentmodel.CollectorCluster
	Namespace                            *agentmodel.Namespace
	NamespaceParentCollector             *agentmodel.CollectorNamespace
	Job                                  *agentmodel.Job
	JobParentCollector                   *agentmodel.CollectorJob
	CronJob                              *agentmodel.CronJob
	CronJobParentCollector               *agentmodel.CollectorCronJob
	DaemonSet                            *agentmodel.DaemonSet
	DaemonSetParentCollector             *agentmodel.CollectorDaemonSet
	StatefulSet                          *agentmodel.StatefulSet
	StatefulSetParentCollector           *agentmodel.CollectorStatefulSet
	PersistentVolume                     *agentmodel.PersistentVolume
	PersistentVolumeParentCollector      *agentmodel.CollectorPersistentVolume
	PersistentVolumeClaim                *agentmodel.PersistentVolumeClaim
	PersistentVolumeClaimParentCollector *agentmodel.CollectorPersistentVolumeClaim
	Role                                 *agentmodel.Role
	RoleParentCollector                  *agentmodel.CollectorRole
	RoleBinding                          *agentmodel.RoleBinding
	RoleBindingParentCollector           *agentmodel.CollectorRoleBinding
	ClusterRole                          *agentmodel.ClusterRole
	ClusterRoleParentCollector           *agentmodel.CollectorClusterRole
	ClusterRoleBinding                   *agentmodel.ClusterRoleBinding
	ClusterRoleBindingParentCollector    *agentmodel.CollectorClusterRoleBinding
	ServiceAccount                       *agentmodel.ServiceAccount
	ServiceAccountParentCollector        *agentmodel.CollectorServiceAccount
	Ingress                              *agentmodel.Ingress
	IngressParentCollector               *agentmodel.CollectorIngress
	VerticalPodAutoscaler                *agentmodel.VerticalPodAutoscaler
	VerticalPodAutoscalerParentCollector *agentmodel.CollectorVerticalPodAutoscaler
}

func (p OrchestratorPayload) name() string {
	return p.Name
}

// GetTags returns the tags within the payload
func (p OrchestratorPayload) GetTags() []string {
	return p.Tags
}

// GetCollectedTime returns the time that the payload was received by the fake intake
func (p OrchestratorPayload) GetCollectedTime() time.Time {
	return p.CollectedTime
}

// ParseOrchestratorPayload parses an api.Payload into a list of OrchestratorPayload
func ParseOrchestratorPayload(payload api.Payload) ([]*OrchestratorPayload, error) {
	msg, err := agentmodel.DecodeMessage(payload.Data)
	if err != nil {
		return nil, err
	}
	var out []*OrchestratorPayload
	switch body := msg.Body.(type) {
	case *agentmodel.CollectorPod:
		for _, pod := range body.Pods {
			out = append(out, &OrchestratorPayload{
				Type:               msg.Header.Type,
				CollectedTime:      payload.Timestamp,
				Pod:                pod,
				PodParentCollector: body,
				UID:                pod.Metadata.Uid,
				Name:               pod.Metadata.Name,
				Tags:               append(body.Tags, pod.Tags...),
			})
		}
	case *agentmodel.CollectorReplicaSet:
		for _, replicaSet := range body.ReplicaSets {
			out = append(out, &OrchestratorPayload{
				Type:                      msg.Header.Type,
				CollectedTime:             payload.Timestamp,
				ReplicaSet:                replicaSet,
				ReplicaSetParentCollector: body,
				UID:                       replicaSet.Metadata.Uid,
				Name:                      replicaSet.Metadata.Name,
				Tags:                      append(body.Tags, replicaSet.Tags...),
			})
		}
	case *agentmodel.CollectorDeployment:
		for _, deployment := range body.Deployments {
			out = append(out, &OrchestratorPayload{
				Type:                      msg.Header.Type,
				CollectedTime:             payload.Timestamp,
				Deployment:                deployment,
				DeploymentParentCollector: body,
				UID:                       deployment.Metadata.Uid,
				Name:                      deployment.Metadata.Name,
				Tags:                      append(body.Tags, deployment.Tags...),
			})
		}
	case *agentmodel.CollectorService:
		for _, service := range body.Services {
			out = append(out, &OrchestratorPayload{
				Type:                   msg.Header.Type,
				CollectedTime:          payload.Timestamp,
				Service:                service,
				ServiceParentCollector: body,
				UID:                    service.Metadata.Uid,
				Name:                   service.Metadata.Name,
				Tags:                   append(body.Tags, service.Tags...),
			})
		}
	case *agentmodel.CollectorNode:
		for _, node := range body.Nodes {
			out = append(out, &OrchestratorPayload{
				Type:                msg.Header.Type,
				CollectedTime:       payload.Timestamp,
				Node:                node,
				NodeParentCollector: body,
				UID:                 node.Metadata.Uid,
				Name:                node.Metadata.Name,
				Tags:                append(body.Tags, node.Tags...),
			})
		}
	case *agentmodel.CollectorCluster:
		out = append(out, &OrchestratorPayload{
			Type:                   msg.Header.Type,
			CollectedTime:          payload.Timestamp,
			Cluster:                body.Cluster,
			ClusterParentCollector: body,
			Name:                   body.ClusterName,
			Tags:                   body.Tags,
		})
	case *agentmodel.CollectorNamespace:
		for _, namespace := range body.Namespaces {
			out = append(out, &OrchestratorPayload{
				Type:                     msg.Header.Type,
				CollectedTime:            payload.Timestamp,
				Namespace:                namespace,
				NamespaceParentCollector: body,
				UID:                      namespace.Metadata.Uid,
				Name:                     namespace.Metadata.Name,
				Tags:                     append(body.Tags, namespace.Tags...),
			})
		}
	case *agentmodel.CollectorJob:
		for _, job := range body.Jobs {
			out = append(out, &OrchestratorPayload{
				Type:               msg.Header.Type,
				CollectedTime:      payload.Timestamp,
				Job:                job,
				JobParentCollector: body,
				UID:                job.Metadata.Uid,
				Name:               job.Metadata.Name,
				Tags:               append(body.Tags, job.Tags...),
			})
		}
	case *agentmodel.CollectorCronJob:
		for _, cronJob := range body.CronJobs {
			out = append(out, &OrchestratorPayload{
				Type:                   msg.Header.Type,
				CollectedTime:          payload.Timestamp,
				CronJob:                cronJob,
				CronJobParentCollector: body,
				UID:                    cronJob.Metadata.Uid,
				Name:                   cronJob.Metadata.Name,
				Tags:                   append(body.Tags, cronJob.Tags...),
			})
		}
	case *agentmodel.CollectorDaemonSet:
		for _, daemonSet := range body.DaemonSets {
			out = append(out, &OrchestratorPayload{
				Type:                     msg.Header.Type,
				CollectedTime:            payload.Timestamp,
				DaemonSet:                daemonSet,
				DaemonSetParentCollector: body,
				UID:                      daemonSet.Metadata.Uid,
				Name:                     daemonSet.Metadata.Name,
				Tags:                     append(body.Tags, daemonSet.Tags...),
			})
		}
	case *agentmodel.CollectorStatefulSet:
		for _, statefulSet := range body.StatefulSets {
			out = append(out, &OrchestratorPayload{
				Type:                       msg.Header.Type,
				CollectedTime:              payload.Timestamp,
				StatefulSet:                statefulSet,
				StatefulSetParentCollector: body,
				UID:                        statefulSet.Metadata.Uid,
				Name:                       statefulSet.Metadata.Name,
				Tags:                       append(body.Tags, statefulSet.Tags...),
			})
		}
	case *agentmodel.CollectorPersistentVolume:
		for _, persistentVolume := range body.PersistentVolumes {
			out = append(out, &OrchestratorPayload{
				Type:                            msg.Header.Type,
				CollectedTime:                   payload.Timestamp,
				PersistentVolume:                persistentVolume,
				PersistentVolumeParentCollector: body,
				UID:                             persistentVolume.Metadata.Uid,
				Name:                            persistentVolume.Metadata.Name,
				Tags:                            append(body.Tags, persistentVolume.Tags...),
			})
		}
	case *agentmodel.CollectorPersistentVolumeClaim:
		for _, persistentVolumeClaim := range body.PersistentVolumeClaims {
			out = append(out, &OrchestratorPayload{
				Type:                                 msg.Header.Type,
				CollectedTime:                        payload.Timestamp,
				PersistentVolumeClaim:                persistentVolumeClaim,
				PersistentVolumeClaimParentCollector: body,
				UID:                                  persistentVolumeClaim.Metadata.Uid,
				Name:                                 persistentVolumeClaim.Metadata.Name,
				Tags:                                 append(body.Tags, persistentVolumeClaim.Tags...),
			})
		}
	case *agentmodel.CollectorRole:
		for _, role := range body.Roles {
			out = append(out, &OrchestratorPayload{
				Type:                msg.Header.Type,
				CollectedTime:       payload.Timestamp,
				Role:                role,
				RoleParentCollector: body,
				UID:                 role.Metadata.Uid,
				Name:                role.Metadata.Name,
				Tags:                append(body.Tags, role.Tags...),
			})
		}
	case *agentmodel.CollectorRoleBinding:
		for _, roleBinding := range body.RoleBindings {
			out = append(out, &OrchestratorPayload{
				Type:                       msg.Header.Type,
				CollectedTime:              payload.Timestamp,
				RoleBinding:                roleBinding,
				RoleBindingParentCollector: body,
				UID:                        roleBinding.Metadata.Uid,
				Name:                       roleBinding.Metadata.Name,
				Tags:                       append(body.Tags, roleBinding.Tags...),
			})
		}
	case *agentmodel.CollectorClusterRole:
		for _, clusterRole := range body.ClusterRoles {
			out = append(out, &OrchestratorPayload{
				Type:                       msg.Header.Type,
				CollectedTime:              payload.Timestamp,
				ClusterRole:                clusterRole,
				ClusterRoleParentCollector: body,
				UID:                        clusterRole.Metadata.Uid,
				Name:                       clusterRole.Metadata.Name,
				Tags:                       append(body.Tags, clusterRole.Tags...),
			})
		}
	case *agentmodel.CollectorClusterRoleBinding:
		for _, clusterRoleBinding := range body.ClusterRoleBindings {
			out = append(out, &OrchestratorPayload{
				Type:                              msg.Header.Type,
				CollectedTime:                     payload.Timestamp,
				ClusterRoleBinding:                clusterRoleBinding,
				ClusterRoleBindingParentCollector: body,
				UID:                               clusterRoleBinding.Metadata.Uid,
				Name:                              clusterRoleBinding.Metadata.Name,
				Tags:                              append(body.Tags, clusterRoleBinding.Tags...),
			})
		}
	case *agentmodel.CollectorServiceAccount:
		for _, serviceAccount := range body.ServiceAccounts {
			out = append(out, &OrchestratorPayload{
				Type:                          msg.Header.Type,
				CollectedTime:                 payload.Timestamp,
				ServiceAccount:                serviceAccount,
				ServiceAccountParentCollector: body,
				UID:                           serviceAccount.Metadata.Uid,
				Name:                          serviceAccount.Metadata.Name,
				Tags:                          append(body.Tags, serviceAccount.Tags...),
			})
		}
	case *agentmodel.CollectorIngress:
		for _, ingress := range body.Ingresses {
			out = append(out, &OrchestratorPayload{
				Type:                   msg.Header.Type,
				CollectedTime:          payload.Timestamp,
				Ingress:                ingress,
				IngressParentCollector: body,
				UID:                    ingress.Metadata.Uid,
				Name:                   ingress.Metadata.Name,
				Tags:                   append(body.Tags, ingress.Tags...),
			})
		}
	case *agentmodel.CollectorVerticalPodAutoscaler:
		for _, verticalPodAutoscaler := range body.VerticalPodAutoscalers {
			out = append(out, &OrchestratorPayload{
				Type:                                 msg.Header.Type,
				CollectedTime:                        payload.Timestamp,
				VerticalPodAutoscaler:                verticalPodAutoscaler,
				VerticalPodAutoscalerParentCollector: body,
				UID:                                  verticalPodAutoscaler.Metadata.Uid,
				Name:                                 verticalPodAutoscaler.Metadata.Name,
				Tags:                                 append(body.Tags, verticalPodAutoscaler.Tags...),
			})
		}

	default:
		return nil, fmt.Errorf("unexpected type %s", msg.Header.Type)
	}

	return out, nil
}

// OrchestratorAggregator is an Aggregator for OrchestratorPayload
type OrchestratorAggregator struct {
	Aggregator[*OrchestratorPayload]
}

// NewOrchestratorAggregator returns a new OrchestratorAggregator
func NewOrchestratorAggregator() OrchestratorAggregator {
	return OrchestratorAggregator{
		Aggregator: newAggregator(ParseOrchestratorPayload),
	}
}

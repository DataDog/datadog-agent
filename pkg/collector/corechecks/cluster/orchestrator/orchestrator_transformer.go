// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	"fmt"
	"strings"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/util/orchestrator"
	orchutil "github.com/DataDog/datadog-agent/pkg/util/orchestrator"

	v1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func extractDeployment(d *v1.Deployment) *model.Deployment {
	deploy := model.Deployment{
		Metadata: orchutil.ExtractMetadata(&d.ObjectMeta),
	}
	// spec
	deploy.ReplicasDesired = 1 // default
	if d.Spec.Replicas != nil {
		deploy.ReplicasDesired = *d.Spec.Replicas
	}
	deploy.Paused = d.Spec.Paused
	deploy.DeploymentStrategy = string(d.Spec.Strategy.Type)
	if deploy.DeploymentStrategy == "RollingUpdate" && d.Spec.Strategy.RollingUpdate != nil {
		if d.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			deploy.MaxUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal
		}
		if d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			deploy.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.StrVal
		}
	}
	if d.Spec.Selector != nil {
		deploy.Selectors = extractLabelSelector(d.Spec.Selector)
	}

	// status
	deploy.Replicas = d.Status.Replicas
	deploy.UpdatedReplicas = d.Status.UpdatedReplicas
	deploy.ReadyReplicas = d.Status.ReadyReplicas
	deploy.AvailableReplicas = d.Status.AvailableReplicas
	deploy.UnavailableReplicas = d.Status.UnavailableReplicas
	deploy.ConditionMessage = extractDeploymentConditionMessage(d.Status.Conditions)

	return &deploy
}

func extractJob(j *batchv1.Job) *model.Job {
	job := model.Job{
		Metadata: orchutil.ExtractMetadata(&j.ObjectMeta),
		Spec:     &model.JobSpec{},
		Status: &model.JobStatus{
			Active:           j.Status.Active,
			ConditionMessage: extractJobConditionMessage(j.Status.Conditions),
			Failed:           j.Status.Failed,
			Succeeded:        j.Status.Succeeded,
		},
	}

	if j.Spec.ActiveDeadlineSeconds != nil {
		job.Spec.ActiveDeadlineSeconds = *j.Spec.ActiveDeadlineSeconds
	}
	if j.Spec.BackoffLimit != nil {
		job.Spec.BackoffLimit = *j.Spec.BackoffLimit
	}
	if j.Spec.Completions != nil {
		job.Spec.Completions = *j.Spec.Completions
	}
	if j.Spec.ManualSelector != nil {
		job.Spec.ManualSelector = *j.Spec.ManualSelector
	}
	if j.Spec.Parallelism != nil {
		job.Spec.Parallelism = *j.Spec.Parallelism
	}
	if j.Spec.Selector != nil {
		job.Spec.Selectors = extractLabelSelector(j.Spec.Selector)
	}

	if j.Status.StartTime != nil {
		job.Status.StartTime = j.Status.StartTime.Unix()
	}
	if j.Status.CompletionTime != nil {
		job.Status.CompletionTime = j.Status.CompletionTime.Unix()
	}

	return &job
}

func extractCronJob(cj *batchv1beta1.CronJob) *model.CronJob {
	cronJob := model.CronJob{
		Metadata: orchutil.ExtractMetadata(&cj.ObjectMeta),
		Spec: &model.CronJobSpec{
			ConcurrencyPolicy: string(cj.Spec.ConcurrencyPolicy),
			Schedule:          cj.Spec.Schedule,
		},
		Status: &model.CronJobStatus{},
	}

	if cj.Spec.FailedJobsHistoryLimit != nil {
		cronJob.Spec.FailedJobsHistoryLimit = *cj.Spec.FailedJobsHistoryLimit
	}
	if cj.Spec.StartingDeadlineSeconds != nil {
		cronJob.Spec.StartingDeadlineSeconds = *cj.Spec.StartingDeadlineSeconds
	}
	if cj.Spec.SuccessfulJobsHistoryLimit != nil {
		cronJob.Spec.SuccessfulJobsHistoryLimit = *cj.Spec.SuccessfulJobsHistoryLimit
	}
	if cj.Spec.Suspend != nil {
		cronJob.Spec.Suspend = *cj.Spec.Suspend
	}

	if cj.Status.LastScheduleTime != nil {
		cronJob.Status.LastScheduleTime = cj.Status.LastScheduleTime.Unix()
	}
	for _, job := range cj.Status.Active {
		cronJob.Status.Active = append(cronJob.Status.Active, &model.ObjectReference{
			ApiVersion:      job.APIVersion,
			FieldPath:       job.FieldPath,
			Kind:            job.Kind,
			Name:            job.Name,
			Namespace:       job.Namespace,
			ResourceVersion: job.ResourceVersion,
			Uid:             string(job.UID),
		})
	}

	return &cronJob
}

func extractStatefulSet(sts *v1.StatefulSet) *model.StatefulSet {
	statefulSet := model.StatefulSet{
		Metadata: orchutil.ExtractMetadata(&sts.ObjectMeta),
		Spec: &model.StatefulSetSpec{
			ServiceName:         sts.Spec.ServiceName,
			PodManagementPolicy: string(sts.Spec.PodManagementPolicy),
			UpdateStrategy:      string(sts.Spec.UpdateStrategy.Type),
		},
		Status: &model.StatefulSetStatus{
			Replicas:        sts.Status.Replicas,
			ReadyReplicas:   sts.Status.ReadyReplicas,
			CurrentReplicas: sts.Status.CurrentReplicas,
			UpdatedReplicas: sts.Status.UpdatedReplicas,
		},
	}

	if sts.Spec.UpdateStrategy.Type == "RollingUpdate" && sts.Spec.UpdateStrategy.RollingUpdate != nil {
		if sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			statefulSet.Spec.Partition = *sts.Spec.UpdateStrategy.RollingUpdate.Partition
		}
	}

	if sts.Spec.Replicas != nil {
		statefulSet.Spec.DesiredReplicas = *sts.Spec.Replicas
	}

	if sts.Spec.Selector != nil {
		statefulSet.Spec.Selectors = extractLabelSelector(sts.Spec.Selector)
	}

	return &statefulSet
}

func extractDaemonSet(ds *v1.DaemonSet) *model.DaemonSet {
	daemonSet := model.DaemonSet{
		Metadata: orchutil.ExtractMetadata(&ds.ObjectMeta),
		Spec: &model.DaemonSetSpec{
			MinReadySeconds: ds.Spec.MinReadySeconds,
		},
		Status: &model.DaemonSetStatus{
			CurrentNumberScheduled: ds.Status.CurrentNumberScheduled,
			NumberMisscheduled:     ds.Status.NumberMisscheduled,
			DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
			NumberReady:            ds.Status.NumberReady,
			UpdatedNumberScheduled: ds.Status.UpdatedNumberScheduled,
			NumberAvailable:        ds.Status.NumberAvailable,
			NumberUnavailable:      ds.Status.NumberUnavailable,
		},
	}

	if ds.Spec.RevisionHistoryLimit != nil {
		daemonSet.Spec.RevisionHistoryLimit = *ds.Spec.RevisionHistoryLimit
	}

	daemonSet.Spec.DeploymentStrategy = string(ds.Spec.UpdateStrategy.Type)
	if ds.Spec.UpdateStrategy.Type == "RollingUpdate" && ds.Spec.UpdateStrategy.RollingUpdate != nil {
		if ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
			daemonSet.Spec.MaxUnavailable = ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable.StrVal
		}
	}

	if ds.Spec.Selector != nil {
		daemonSet.Spec.Selectors = extractLabelSelector(ds.Spec.Selector)
	}

	return &daemonSet
}

func extractReplicaSet(rs *v1.ReplicaSet) *model.ReplicaSet {
	replicaSet := model.ReplicaSet{
		Metadata: orchutil.ExtractMetadata(&rs.ObjectMeta),
	}
	// spec
	replicaSet.ReplicasDesired = 1 // default
	if rs.Spec.Replicas != nil {
		replicaSet.ReplicasDesired = *rs.Spec.Replicas
	}
	if rs.Spec.Selector != nil {
		replicaSet.Selectors = extractLabelSelector(rs.Spec.Selector)
	}

	// status
	replicaSet.Replicas = rs.Status.Replicas
	replicaSet.FullyLabeledReplicas = rs.Status.FullyLabeledReplicas
	replicaSet.ReadyReplicas = rs.Status.ReadyReplicas
	replicaSet.AvailableReplicas = rs.Status.AvailableReplicas

	return &replicaSet
}

func extractRole(r *rbacv1.Role) *model.Role {
	return &model.Role{
		Metadata: orchestrator.ExtractMetadata(&r.ObjectMeta),
		Rules:    extractPolicyRules(r.Rules),
	}
}

func extractPolicyRules(r []rbacv1.PolicyRule) []*model.PolicyRule {
	rules := make([]*model.PolicyRule, 0, len(r))
	for _, rule := range r {
		rules = append(rules, &model.PolicyRule{
			ApiGroups:       rule.APIGroups,
			NonResourceURLs: rule.NonResourceURLs,
			Resources:       rule.Resources,
			ResourceNames:   rule.ResourceNames,
			Verbs:           rule.Verbs,
		})
	}
	return rules
}

func extractRoleBinding(rb *rbacv1.RoleBinding) *model.RoleBinding {
	return &model.RoleBinding{
		Metadata: orchestrator.ExtractMetadata(&rb.ObjectMeta),
		RoleRef:  extractRoleRef(&rb.RoleRef),
		Subjects: extractSubjects(rb.Subjects),
	}
}

func extractRoleRef(r *rbacv1.RoleRef) *model.TypedLocalObjectReference {
	return &model.TypedLocalObjectReference{
		ApiGroup: r.APIGroup,
		Kind:     r.Kind,
		Name:     r.Name,
	}
}

func extractSubjects(s []rbacv1.Subject) []*model.Subject {
	subjects := make([]*model.Subject, 0, len(s))
	for _, subject := range s {
		subjects = append(subjects, &model.Subject{
			ApiGroup:  subject.APIGroup,
			Kind:      subject.Kind,
			Name:      subject.Name,
			Namespace: subject.Namespace,
		})
	}
	return subjects
}

func extractClusterRole(cr *rbacv1.ClusterRole) *model.ClusterRole {
	clusterRole := &model.ClusterRole{
		Metadata: orchestrator.ExtractMetadata(&cr.ObjectMeta),
		Rules:    extractPolicyRules(cr.Rules),
	}
	if cr.AggregationRule != nil {
		for _, rule := range cr.AggregationRule.ClusterRoleSelectors {
			clusterRole.AggregationRules = append(clusterRole.AggregationRules, extractLabelSelector(&rule)...)
		}
	}
	return clusterRole
}

func extractClusterRoleBinding(crb *rbacv1.ClusterRoleBinding) *model.ClusterRoleBinding {
	return &model.ClusterRoleBinding{
		Metadata: orchestrator.ExtractMetadata(&crb.ObjectMeta),
		RoleRef:  extractRoleRef(&crb.RoleRef),
		Subjects: extractSubjects(crb.Subjects),
	}
}

func extractServiceAccount(sa *corev1.ServiceAccount) *model.ServiceAccount {
	serviceAccount := &model.ServiceAccount{
		Metadata: orchestrator.ExtractMetadata(&sa.ObjectMeta),
	}
	if sa.AutomountServiceAccountToken != nil {
		serviceAccount.AutomountServiceAccountToken = *sa.AutomountServiceAccountToken
	}
	// Extract secret references.
	for _, secret := range sa.Secrets {
		serviceAccount.Secrets = append(serviceAccount.Secrets, &model.ObjectReference{
			ApiVersion:      secret.APIVersion,
			FieldPath:       secret.FieldPath,
			Kind:            secret.Kind,
			Name:            secret.Name,
			Namespace:       secret.Namespace,
			ResourceVersion: secret.ResourceVersion,
			Uid:             string(secret.UID),
		})
	}
	// Extract secret references for pulling images.
	for _, imgPullSecret := range sa.ImagePullSecrets {
		serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, &model.TypedLocalObjectReference{
			Name: imgPullSecret.Name,
		})
	}
	return serviceAccount
}

// extractService returns the protobuf Service message corresponding to
// a Kubernetes service object.
func extractService(s *corev1.Service) *model.Service {
	message := &model.Service{
		Metadata: orchutil.ExtractMetadata(&s.ObjectMeta),
		Spec: &model.ServiceSpec{
			ExternalIPs:              s.Spec.ExternalIPs,
			ExternalTrafficPolicy:    string(s.Spec.ExternalTrafficPolicy),
			PublishNotReadyAddresses: s.Spec.PublishNotReadyAddresses,
			SessionAffinity:          string(s.Spec.SessionAffinity),
			Type:                     string(s.Spec.Type),
		},
		Status: &model.ServiceStatus{},
	}

	if s.Spec.IPFamilies != nil {
		strFamilies := make([]string, len(s.Spec.IPFamilies))
		for i, fam := range s.Spec.IPFamilies {
			strFamilies[i] = string(fam)
		}
		message.Spec.IpFamily = strings.Join(strFamilies, ", ")
	}
	if s.Spec.SessionAffinityConfig != nil && s.Spec.SessionAffinityConfig.ClientIP != nil {
		message.Spec.SessionAffinityConfig = &model.ServiceSessionAffinityConfig{
			ClientIPTimeoutSeconds: *s.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds,
		}
	}
	if s.Spec.Type == corev1.ServiceTypeExternalName {
		message.Spec.ExternalName = s.Spec.ExternalName
	} else {
		message.Spec.ClusterIP = s.Spec.ClusterIP
	}
	if s.Spec.Type == corev1.ServiceTypeLoadBalancer {
		message.Spec.LoadBalancerIP = s.Spec.LoadBalancerIP
		message.Spec.LoadBalancerSourceRanges = s.Spec.LoadBalancerSourceRanges

		if s.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal {
			message.Spec.HealthCheckNodePort = s.Spec.HealthCheckNodePort
		}

		for _, ingress := range s.Status.LoadBalancer.Ingress {
			if ingress.Hostname != "" {
				message.Status.LoadBalancerIngress = append(message.Status.LoadBalancerIngress, ingress.Hostname)
			} else if ingress.IP != "" {
				message.Status.LoadBalancerIngress = append(message.Status.LoadBalancerIngress, ingress.IP)
			}
		}
	}

	if s.Spec.Selector != nil {
		message.Spec.Selectors = extractServiceSelector(s.Spec.Selector)
	}

	for _, port := range s.Spec.Ports {
		message.Spec.Ports = append(message.Spec.Ports, &model.ServicePort{
			Name:       port.Name,
			Protocol:   string(port.Protocol),
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
			NodePort:   port.NodePort,
		})
	}

	return message
}

func extractServiceSelector(ls map[string]string) []*model.LabelSelectorRequirement {
	labelSelectors := make([]*model.LabelSelectorRequirement, 0, len(ls))
	for k, v := range ls {
		labelSelectors = append(labelSelectors, &model.LabelSelectorRequirement{
			Key:      k,
			Operator: "In",
			Values:   []string{v},
		})
	}
	return labelSelectors
}

// extractPersistentVolume returns the protobuf Persistent Volume message corresponding to
// a Kubernetes persistent volume object.
func extractPersistentVolume(pv *corev1.PersistentVolume) *model.PersistentVolume {
	message := &model.PersistentVolume{
		Metadata: orchutil.ExtractMetadata(&pv.ObjectMeta),
		Spec: &model.PersistentVolumeSpec{
			Capacity:                      map[string]int64{},
			PersistentVolumeReclaimPolicy: string(pv.Spec.PersistentVolumeReclaimPolicy),
			StorageClassName:              pv.Spec.StorageClassName,
			MountOptions:                  pv.Spec.MountOptions,
		},
		Status: &model.PersistentVolumeStatus{
			Phase:   string(pv.Status.Phase),
			Message: pv.Status.Message,
			Reason:  pv.Status.Reason,
		},
	}

	if pv.Spec.VolumeMode != nil {
		message.Spec.VolumeMode = string(*pv.Spec.VolumeMode)
	}

	modes := pv.Spec.AccessModes
	if len(modes) > 0 {
		ams := make([]string, len(modes))
		for i, mode := range modes {
			ams[i] = string(mode)
		}
		message.Spec.AccessModes = ams
	}

	claimRef := pv.Spec.ClaimRef
	if claimRef != nil {
		message.Spec.ClaimRef = &model.ObjectReference{
			Kind:            claimRef.Kind,
			Namespace:       claimRef.Namespace,
			Name:            claimRef.Name,
			Uid:             string(claimRef.UID),
			ApiVersion:      claimRef.APIVersion,
			ResourceVersion: claimRef.ResourceVersion,
			FieldPath:       claimRef.FieldPath,
		}
	}

	nodeAffinity := pv.Spec.NodeAffinity
	if nodeAffinity != nil {
		selectorTerms := make([]*model.NodeSelectorTerm, len(nodeAffinity.Required.NodeSelectorTerms))
		terms := nodeAffinity.Required.NodeSelectorTerms
		for i, term := range terms {
			selectorTerms[i] = &model.NodeSelectorTerm{
				MatchExpressions: extractPVSelector(term.MatchExpressions),
				MatchFields:      extractPVSelector(term.MatchFields),
			}
		}
		message.Spec.NodeAffinity = selectorTerms
	}

	message.Spec.PersistentVolumeType = extractVolumeSource(pv.Spec.PersistentVolumeSource)

	st := pv.Spec.Capacity.Storage()
	if !st.IsZero() {
		message.Spec.Capacity[corev1.ResourceStorage.String()] = st.Value()
	}
	return message
}

func extractVolumeSource(volume corev1.PersistentVolumeSource) string {
	switch {
	case volume.HostPath != nil:
		return "HostPath"
	case volume.GCEPersistentDisk != nil:
		return "GCEPersistentDisk"
	case volume.AWSElasticBlockStore != nil:
		return "AWSElasticBlockStore"
	case volume.Quobyte != nil:
		return "Quobyte"
	case volume.Cinder != nil:
		return "Cinder"
	case volume.PhotonPersistentDisk != nil:
		return "PhotonPersistentDisk"
	case volume.PortworxVolume != nil:
		return "PortworxVolume"
	case volume.ScaleIO != nil:
		return "ScaleIO"
	case volume.CephFS != nil:
		return "CephFS"
	case volume.StorageOS != nil:
		return "StorageOS"
	case volume.FC != nil:
		return "FC"
	case volume.AzureFile != nil:
		return "AzureFile"
	case volume.FlexVolume != nil:
		return "FlexVolume"
	case volume.Flocker != nil:
		return "Flocker"
	case volume.CSI != nil:
		return "CSI"
	}
	return "<unknown>"
}

func extractPVSelector(ls []corev1.NodeSelectorRequirement) []*model.LabelSelectorRequirement {
	if len(ls) == 0 {
		return nil
	}

	labelSelectors := make([]*model.LabelSelectorRequirement, len(ls))
	for i, v := range ls {
		labelSelectors[i] = &model.LabelSelectorRequirement{
			Key:      v.Key,
			Operator: string(v.Operator),
			Values:   v.Values,
		}
	}
	return labelSelectors
}

// extractPersistentVolumeClaim returns the protobuf Persistent Volume Claim message corresponding to
// a Kubernetes persistent volume claim object.
func extractPersistentVolumeClaim(pvc *corev1.PersistentVolumeClaim) *model.PersistentVolumeClaim {
	message := &model.PersistentVolumeClaim{
		Metadata: orchutil.ExtractMetadata(&pvc.ObjectMeta),
		Spec: &model.PersistentVolumeClaimSpec{
			VolumeName: pvc.Spec.VolumeName,
			Resources:  &model.ResourceRequirements{},
		},
		Status: &model.PersistentVolumeClaimStatus{
			Phase:    string(pvc.Status.Phase),
			Capacity: map[string]int64{},
		},
	}
	extractSpec(pvc, message)
	extractStatus(pvc, message)

	return message
}

func extractStatus(pvc *corev1.PersistentVolumeClaim, message *model.PersistentVolumeClaim) {
	pvcCons := pvc.Status.Conditions
	if len(pvcCons) > 0 {
		cons := make([]*model.PersistentVolumeClaimCondition, len(pvcCons))
		for i, condition := range pvcCons {
			cons[i] = &model.PersistentVolumeClaimCondition{
				Type:    string(condition.Type),
				Status:  string(condition.Status),
				Reason:  condition.Reason,
				Message: condition.Message,
			}
			if !condition.LastProbeTime.IsZero() {
				cons[i].LastProbeTime = condition.LastProbeTime.Unix()
			}
			if !condition.LastTransitionTime.IsZero() {
				cons[i].LastTransitionTime = condition.LastProbeTime.Unix()
			}
		}
		message.Status.Conditions = cons
	}

	if pvc.Status.AccessModes != nil {
		strModes := make([]string, len(pvc.Status.AccessModes))
		for i, am := range pvc.Status.AccessModes {
			strModes[i] = string(am)
		}
		message.Status.AccessModes = strModes
	}

	st := pvc.Status.Capacity.Storage()
	if st != nil && !st.IsZero() {
		message.Status.Capacity[corev1.ResourceStorage.String()] = st.Value()
	}
}

func extractSpec(pvc *corev1.PersistentVolumeClaim, message *model.PersistentVolumeClaim) {
	ds := pvc.Spec.DataSource
	if ds != nil {
		t := &model.TypedLocalObjectReference{Kind: ds.Kind, Name: ds.Name}
		if ds.APIGroup != nil {
			t.ApiGroup = *ds.APIGroup
		}
		message.Spec.DataSource = t
	}

	if pvc.Spec.VolumeMode != nil {
		message.Spec.VolumeMode = string(*pvc.Spec.VolumeMode)
	}

	if pvc.Spec.StorageClassName != nil {
		message.Spec.StorageClassName = *pvc.Spec.StorageClassName
	}

	if pvc.Spec.AccessModes != nil {
		strModes := make([]string, len(pvc.Spec.AccessModes))
		for i, am := range pvc.Spec.AccessModes {
			strModes[i] = string(am)
		}
		message.Spec.AccessModes = strModes
	}

	reSt := pvc.Spec.Resources.Requests.Storage()
	if reSt != nil && !reSt.IsZero() {
		message.Spec.Resources.Requests = map[string]int64{string(corev1.ResourceStorage): reSt.Value()}
	}
	reLt := pvc.Spec.Resources.Limits.Storage()
	if reLt != nil && !reLt.IsZero() {
		message.Spec.Resources.Limits = map[string]int64{string(corev1.ResourceStorage): reLt.Value()}
	}

	if pvc.Spec.Selector != nil {
		message.Spec.Selector = extractLabelSelector(pvc.Spec.Selector)
	}
}

func extractLabelSelector(ls *metav1.LabelSelector) []*model.LabelSelectorRequirement {
	labelSelectors := make([]*model.LabelSelectorRequirement, 0, len(ls.MatchLabels)+len(ls.MatchExpressions))
	for k, v := range ls.MatchLabels {
		s := model.LabelSelectorRequirement{
			Key:      k,
			Operator: "In",
			Values:   []string{v},
		}
		labelSelectors = append(labelSelectors, &s)
	}
	for _, s := range ls.MatchExpressions {
		sr := model.LabelSelectorRequirement{
			Key:      s.Key,
			Operator: string(s.Operator),
			Values:   s.Values,
		}
		labelSelectors = append(labelSelectors, &sr)
	}

	return labelSelectors
}

func extractDeploymentConditionMessage(conditions []v1.DeploymentCondition) string {
	messageMap := make(map[v1.DeploymentConditionType]string)

	// from https://github.com/kubernetes/kubernetes/blob/0b678bbb51a83e47df912f1205907418e354b281/staging/src/k8s.io/api/apps/v1/types.go#L417-L430
	// update if new ones appear
	chronologicalConditions := []v1.DeploymentConditionType{
		v1.DeploymentReplicaFailure,
		v1.DeploymentProgressing,
		v1.DeploymentAvailable,
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range conditions {
		if c.Status == corev1.ConditionFalse && c.Message != "" {
			messageMap[c.Type] = c.Message
		}
	}

	// return the message of the first one that failed
	for _, c := range chronologicalConditions {
		if m := messageMap[c]; m != "" {
			return m
		}
	}
	return ""
}

func extractJobConditionMessage(conditions []batchv1.JobCondition) string {
	for _, c := range conditions {
		if c.Type == batchv1.JobFailed && c.Message != "" {
			return c.Message
		}
	}
	return ""
}

func extractNode(n *corev1.Node) *model.Node {
	msg := &model.Node{
		Metadata:      orchutil.ExtractMetadata(&n.ObjectMeta),
		PodCIDR:       n.Spec.PodCIDR,
		PodCIDRs:      n.Spec.PodCIDRs,
		ProviderID:    n.Spec.ProviderID,
		Unschedulable: n.Spec.Unschedulable,
		Status: &model.NodeStatus{
			Allocatable:             map[string]int64{},
			Capacity:                map[string]int64{},
			Architecture:            n.Status.NodeInfo.Architecture,
			ContainerRuntimeVersion: n.Status.NodeInfo.ContainerRuntimeVersion,
			OperatingSystem:         n.Status.NodeInfo.OperatingSystem,
			OsImage:                 n.Status.NodeInfo.OSImage,
			KernelVersion:           n.Status.NodeInfo.KernelVersion,
			KubeletVersion:          n.Status.NodeInfo.KubeletVersion,
			KubeProxyVersion:        n.Status.NodeInfo.KubeProxyVersion,
		},
	}

	if len(n.Spec.Taints) > 0 {
		msg.Taints = extractTaints(n.Spec.Taints)
	}

	extractCapacitiesAndAllocatables(n, msg)

	// extract status addresses
	if len(n.Status.Addresses) > 0 {
		msg.Status.NodeAddresses = map[string]string{}
		for _, address := range n.Status.Addresses {
			msg.Status.NodeAddresses[string(address.Type)] = address.Address
		}
	}

	// extract conditions
	for _, condition := range n.Status.Conditions {
		c := &model.NodeCondition{
			Type:    string(condition.Type),
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		}
		if !condition.LastTransitionTime.IsZero() {
			c.LastTransitionTime = condition.LastTransitionTime.Unix()
		}
		msg.Status.Conditions = append(msg.Status.Conditions, c)
	}

	// extract status message
	msg.Status.Status = computeNodeStatus(n)

	// extract role
	roles := findNodeRoles(n.Labels)
	if len(roles) > 0 {
		msg.Roles = roles
	}

	for _, image := range n.Status.Images {
		msg.Status.Images = append(msg.Status.Images, &model.ContainerImage{
			Names:     image.Names,
			SizeBytes: image.SizeBytes,
		})
	}

	return msg
}

func extractCapacitiesAndAllocatables(n *corev1.Node, mn *model.Node) {
	// Milli Value ceil(q * 1000), which fits to be the lowest value. CPU -> Millicore and Memory -> byte
	supportedResourcesMilli := []corev1.ResourceName{corev1.ResourceCPU}
	supportedResources := []corev1.ResourceName{corev1.ResourcePods, corev1.ResourceMemory}
	setSupportedResources(n, mn, supportedResources, false)
	setSupportedResources(n, mn, supportedResourcesMilli, true)
}

func setSupportedResources(n *corev1.Node, mn *model.Node, supportedResources []corev1.ResourceName, isMilli bool) {
	for _, resource := range supportedResources {
		capacity, hasCapacity := n.Status.Capacity[resource]
		if hasCapacity && !capacity.IsZero() {
			if isMilli {
				mn.Status.Capacity[resource.String()] = capacity.MilliValue()
			} else {
				mn.Status.Capacity[resource.String()] = capacity.Value()
			}
		}
		allocatable, hasAllocatable := n.Status.Allocatable[resource]

		if hasAllocatable && !allocatable.IsZero() {
			if isMilli {
				mn.Status.Allocatable[resource.String()] = allocatable.MilliValue()
			} else {
				mn.Status.Allocatable[resource.String()] = allocatable.Value()
			}
		}
	}
}

func extractTaints(taints []corev1.Taint) []*model.Taint {
	modelTaints := make([]*model.Taint, 0, len(taints))

	for _, taint := range taints {
		modelTaint := &model.Taint{
			Key:    taint.Key,
			Value:  taint.Value,
			Effect: string(taint.Effect),
		}
		if !taint.TimeAdded.IsZero() {
			modelTaint.TimeAdded = taint.TimeAdded.Unix()
		}
		modelTaints = append(modelTaints, modelTaint)
	}
	return modelTaints
}

// computeNodeStatus is mostly copied from kubernetes to match what users see in kubectl
// in case of issues, check for changes upstream: https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L1410
func computeNodeStatus(n *corev1.Node) string {
	conditionMap := make(map[corev1.NodeConditionType]*corev1.NodeCondition)
	NodeAllConditions := []corev1.NodeConditionType{corev1.NodeReady}
	for i := range n.Status.Conditions {
		cond := n.Status.Conditions[i]
		conditionMap[cond.Type] = &cond
	}
	var status []string
	for _, validCondition := range NodeAllConditions {
		if condition, ok := conditionMap[validCondition]; ok {
			if condition.Status == corev1.ConditionTrue {
				status = append(status, string(condition.Type))
			} else {
				status = append(status, "Not"+string(condition.Type))
			}
		}
	}
	if len(status) == 0 {
		status = append(status, "Unknown")
	}
	if n.Spec.Unschedulable {
		status = append(status, "SchedulingDisabled")
	}
	return strings.Join(status, ",")
}

func convertNodeStatusToTags(nodeStatus string) []string {
	var tags []string
	unschedulable := false
	for _, status := range strings.Split(nodeStatus, ",") {
		if status == "" {
			continue
		}
		if status == "SchedulingDisabled" {
			unschedulable = true
			tags = append(tags, "node_schedulable:false")
			continue
		}
		tags = append(tags, fmt.Sprintf("node_status:%s", strings.ToLower(status)))
	}
	if !unschedulable {
		tags = append(tags, "node_schedulable:true")
	}
	return tags
}

// findNodeRoles returns the roles of a given node.
// The roles are determined by looking for:
// * a node-role.kubernetes.io/<role>="" label
// * a kubernetes.io/role="<role>" label
// is mostly copied from kubernetes, for issues check upstream: https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L1487
func findNodeRoles(nodeLabels map[string]string) []string {
	labelNodeRolePrefix := "node-role.kubernetes.io/"
	nodeLabelRole := "kubernetes.io/role"

	roles := sets.NewString()
	for k, v := range nodeLabels {
		switch {
		case strings.HasPrefix(k, labelNodeRolePrefix):
			if role := strings.TrimPrefix(k, labelNodeRolePrefix); len(role) > 0 {
				roles.Insert(role)
			}

		case k == nodeLabelRole && v != "":
			roles.Insert(v)
		}
	}
	return roles.List()
}

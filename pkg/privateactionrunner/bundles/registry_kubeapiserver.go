// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// This file provides the bundle registry used inside the DD cluster agent.
package privatebundles

import (
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	com_datadoghq_ddagent "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/ddagent"
	com_datadoghq_ddagent_networkpath "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/ddagent/networkpath"
	com_datadoghq_ddagent_shell "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/ddagent/shell"
	com_datadoghq_gitlab_branches "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/branches"
	com_datadoghq_gitlab_commits "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/commits"
	com_datadoghq_gitlab_customattributes "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/customattributes"
	com_datadoghq_gitlab_deployments "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/deployments"
	com_datadoghq_gitlab_environments "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/environments"
	com_datadoghq_gitlab_graphql "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/graphql"
	com_datadoghq_gitlab_groups "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/groups"
	com_datadoghq_gitlab_issues "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/issues"
	com_datadoghq_gitlab_jobs "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/jobs"
	com_datadoghq_gitlab_labels "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/labels"
	com_datadoghq_gitlab_members "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/members"
	com_datadoghq_gitlab_merge_requests "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/mergerequests"
	com_datadoghq_gitlab_notes "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/notes"
	com_datadoghq_gitlab_pipelines "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/pipelines"
	com_datadoghq_gitlab_projects "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/projects"
	com_datadoghq_gitlab_protected_branches "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/protectedbranches"
	com_datadoghq_gitlab_repositories "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/repositories"
	com_datadoghq_gitlab_repository_files "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/repositoryfiles"
	com_datadoghq_gitlab_tags "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/tags"
	com_datadoghq_gitlab_users "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/gitlab/users"
	com_datadoghq_http "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/http"
	com_datadoghq_jenkins "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/jenkins"
	com_datadoghq_kubernetes_apiextensions "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apiextensions"
	com_datadoghq_kubernetes_apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/apps"
	com_datadoghq_kubernetes_batch "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/batch"
	com_datadoghq_kubernetes_core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/core"
	com_datadoghq_kubernetes_customresources "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/customresources"
	com_datadoghq_kubernetes_discovery "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/kubernetes/discovery"
	com_datadoghq_mongodb "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/mongodb"
	com_datadoghq_script "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/script"
	com_datadoghq_temporal "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/temporal"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Registry struct {
	Bundles map[string]types.Bundle
}

func NewRegistry(configuration *config.Config, traceroute traceroute.Component, eventPlatform eventplatform.Component) *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.ddagent":                    com_datadoghq_ddagent.NewAgentActions(),
			"com.datadoghq.ddagent.networkpath":        com_datadoghq_ddagent_networkpath.NewNetworkPath(traceroute, eventPlatform),
			"com.datadoghq.ddagent.shell":              com_datadoghq_ddagent_shell.NewShellBundle(),
			"com.datadoghq.gitlab.branches":            com_datadoghq_gitlab_branches.NewGitlabBranches(),
			"com.datadoghq.gitlab.commits":             com_datadoghq_gitlab_commits.NewGitlabCommits(),
			"com.datadoghq.gitlab.customattributes":    com_datadoghq_gitlab_customattributes.NewGitlabCustomAttributes(),
			"com.datadoghq.gitlab.deployments":         com_datadoghq_gitlab_deployments.NewGitlabDeployments(),
			"com.datadoghq.gitlab.environments":        com_datadoghq_gitlab_environments.NewGitlabEnvironments(),
			"com.datadoghq.gitlab.graphql":             com_datadoghq_gitlab_graphql.NewGitlabGraphql(),
			"com.datadoghq.gitlab.groups":              com_datadoghq_gitlab_groups.NewGitlabGroups(),
			"com.datadoghq.gitlab.issues":              com_datadoghq_gitlab_issues.NewGitlabIssues(),
			"com.datadoghq.gitlab.jobs":                com_datadoghq_gitlab_jobs.NewGitlabJobs(),
			"com.datadoghq.gitlab.labels":              com_datadoghq_gitlab_labels.NewGitlabLabels(),
			"com.datadoghq.gitlab.members":             com_datadoghq_gitlab_members.NewGitlabMembers(),
			"com.datadoghq.gitlab.mergerequests":       com_datadoghq_gitlab_merge_requests.NewGitlabMergeRequests(),
			"com.datadoghq.gitlab.notes":               com_datadoghq_gitlab_notes.NewGitlabNotes(),
			"com.datadoghq.gitlab.pipelines":           com_datadoghq_gitlab_pipelines.NewGitlabPipelines(),
			"com.datadoghq.gitlab.projects":            com_datadoghq_gitlab_projects.NewGitlabProjects(),
			"com.datadoghq.gitlab.protectedbranches":   com_datadoghq_gitlab_protected_branches.NewGitlabProtectedBranches(),
			"com.datadoghq.gitlab.repositories":        com_datadoghq_gitlab_repositories.NewGitlabRepositories(),
			"com.datadoghq.gitlab.repositoryfiles":     com_datadoghq_gitlab_repository_files.NewGitlabRepositoryFiles(),
			"com.datadoghq.gitlab.tags":                com_datadoghq_gitlab_tags.NewGitlabTags(),
			"com.datadoghq.gitlab.users":               com_datadoghq_gitlab_users.NewGitlabUsers(),
			"com.datadoghq.http":                       com_datadoghq_http.NewHttpBundle(configuration),
			"com.datadoghq.jenkins":                    com_datadoghq_jenkins.NewJenkins(configuration),
			"com.datadoghq.kubernetes.apiextensions":   com_datadoghq_kubernetes_apiextensions.NewKubernetesApiExtensions(),
			"com.datadoghq.kubernetes.apps":            com_datadoghq_kubernetes_apps.NewKubernetesApps(),
			"com.datadoghq.kubernetes.batch":           com_datadoghq_kubernetes_batch.NewKubernetesBatch(),
			"com.datadoghq.kubernetes.core":            com_datadoghq_kubernetes_core.NewKubernetesCore(),
			"com.datadoghq.kubernetes.customresources": com_datadoghq_kubernetes_customresources.NewKubernetesCustomResources(),
			"com.datadoghq.kubernetes.discovery":       com_datadoghq_kubernetes_discovery.NewKubernetesDiscovery(),
			"com.datadoghq.mongodb":                    com_datadoghq_mongodb.NewMongoDB(),
			"com.datadoghq.script":                     com_datadoghq_script.NewScript(),
			"com.datadoghq.temporal":                   com_datadoghq_temporal.NewTemporal(),
		},
	}
}

func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}

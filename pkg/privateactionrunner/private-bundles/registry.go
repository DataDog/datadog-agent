// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privatebundles provides a registry for managing private action bundles.
package privatebundles

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	com_datadoghq_gitlab_branches "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/branches"
	com_datadoghq_gitlab_commits "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/commits"
	com_datadoghq_gitlab_customattributes "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/customattributes"
	com_datadoghq_gitlab_deployments "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/deployments"
	com_datadoghq_gitlab_environments "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/environments"
	com_datadoghq_gitlab_graphql "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/graphql"
	com_datadoghq_gitlab_groups "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/groups"
	com_datadoghq_gitlab_issues "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/issues"
	com_datadoghq_gitlab_jobs "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/jobs"
	com_datadoghq_gitlab_labels "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/labels"
	com_datadoghq_gitlab_members "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/members"
	com_datadoghq_gitlab_merge_requests "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/mergerequests"
	com_datadoghq_gitlab_notes "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/notes"
	com_datadoghq_gitlab_pipelines "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/pipelines"
	com_datadoghq_gitlab_projects "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/projects"
	com_datadoghq_gitlab_protected_branches "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/protectedbranches"
	com_datadoghq_gitlab_repositories "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/repositories"
	com_datadoghq_gitlab_repository_files "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/repositoryfiles"
	com_datadoghq_gitlab_tags "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/tags"
	com_datadoghq_gitlab_users "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/gitlab/users"
	com_datadoghq_jenkins "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/jenkins"
	com_datadoghq_kubernetes_apiextensions "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/apiextensions"
	com_datadoghq_kubernetes_apps "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/apps"
	com_datadoghq_kubernetes_batch "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/batch"
	com_datadoghq_kubernetes_core "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/core"
	com_datadoghq_kubernetes_customresources "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/kubernetes/customresources"
	com_datadoghq_mongodb "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/mongodb"
	com_datadoghq_postgresql "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/postgresql"
	com_datadoghq_script "github.com/DataDog/datadog-agent/pkg/privateactionrunner/private-bundles/com/datadoghq/script"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// Registry manages a collection of private action bundles.
type Registry struct {
	Bundles map[string]types.Bundle
}

// NewRegistry creates a new Registry instance with default bundles.
func NewRegistry(_ *config.Config) *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
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
			"com.datadoghq.jenkins":                    com_datadoghq_jenkins.NewJenkins(),
			"com.datadoghq.kubernetes.apiextensions":   com_datadoghq_kubernetes_apiextensions.NewKubernetesApiExtensions(),
			"com.datadoghq.kubernetes.apps":            com_datadoghq_kubernetes_apps.NewKubernetesApps(),
			"com.datadoghq.kubernetes.batch":           com_datadoghq_kubernetes_batch.NewKubernetesBatch(),
			"com.datadoghq.kubernetes.core":            com_datadoghq_kubernetes_core.NewKubernetesCore(),
			"com.datadoghq.kubernetes.customresources": com_datadoghq_kubernetes_customresources.NewKubernetesCustomResources(),
			"com.datadoghq.mongodb":                    com_datadoghq_mongodb.NewMongoDB(),
			"com.datadoghq.postgresql":                 com_datadoghq_postgresql.NewPostgreSQL(),
			"com.datadoghq.script":                     com_datadoghq_script.NewScript(),
		},
	}
}

// GetBundle returns the bundle with the specified fully qualified name.
func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}

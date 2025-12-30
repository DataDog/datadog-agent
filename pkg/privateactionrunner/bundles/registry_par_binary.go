// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build par_separate_binary

// all actions except kubernetes
package privatebundles

import (
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
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type Registry struct {
	Bundles map[string]types.Bundle
}

func NewRegistry() *Registry {
	return &Registry{
		Bundles: map[string]types.Bundle{
			"com.datadoghq.gitlab.branches":          com_datadoghq_gitlab_branches.NewGitlabBranches(),
			"com.datadoghq.gitlab.commits":           com_datadoghq_gitlab_commits.NewGitlabCommits(),
			"com.datadoghq.gitlab.customattributes":  com_datadoghq_gitlab_customattributes.NewGitlabCustomAttributes(),
			"com.datadoghq.gitlab.deployments":       com_datadoghq_gitlab_deployments.NewGitlabDeployments(),
			"com.datadoghq.gitlab.environments":      com_datadoghq_gitlab_environments.NewGitlabEnvironments(),
			"com.datadoghq.gitlab.graphql":           com_datadoghq_gitlab_graphql.NewGitlabGraphql(),
			"com.datadoghq.gitlab.groups":            com_datadoghq_gitlab_groups.NewGitlabGroups(),
			"com.datadoghq.gitlab.issues":            com_datadoghq_gitlab_issues.NewGitlabIssues(),
			"com.datadoghq.gitlab.jobs":              com_datadoghq_gitlab_jobs.NewGitlabJobs(),
			"com.datadoghq.gitlab.labels":            com_datadoghq_gitlab_labels.NewGitlabLabels(),
			"com.datadoghq.gitlab.members":           com_datadoghq_gitlab_members.NewGitlabMembers(),
			"com.datadoghq.gitlab.mergerequests":     com_datadoghq_gitlab_merge_requests.NewGitlabMergeRequests(),
			"com.datadoghq.gitlab.notes":             com_datadoghq_gitlab_notes.NewGitlabNotes(),
			"com.datadoghq.gitlab.pipelines":         com_datadoghq_gitlab_pipelines.NewGitlabPipelines(),
			"com.datadoghq.gitlab.projects":          com_datadoghq_gitlab_projects.NewGitlabProjects(),
			"com.datadoghq.gitlab.protectedbranches": com_datadoghq_gitlab_protected_branches.NewGitlabProtectedBranches(),
			"com.datadoghq.gitlab.repositories":      com_datadoghq_gitlab_repositories.NewGitlabRepositories(),
			"com.datadoghq.gitlab.repositoryfiles":   com_datadoghq_gitlab_repository_files.NewGitlabRepositoryFiles(),
			"com.datadoghq.gitlab.tags":              com_datadoghq_gitlab_tags.NewGitlabTags(),
			"com.datadoghq.gitlab.users":             com_datadoghq_gitlab_users.NewGitlabUsers(),
		},
	}
}

func (r *Registry) GetBundle(fqn string) types.Bundle {
	return r.Bundles[fqn]
}

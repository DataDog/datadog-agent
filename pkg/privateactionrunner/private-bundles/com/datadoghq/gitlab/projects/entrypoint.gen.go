package com_datadoghq_gitlab_projects

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabProjectsBundle struct {
	actions map[string]types.Action
}

func NewGitlabProjects() types.Bundle {
	return &GitlabProjectsBundle{
		actions: map[string]types.Action{
			// Manual actions
			"createProject":            NewCreateProjectHandler(),
			"createProjectForUser":     NewCreateProjectForUserHandler(),
			"editProject":              NewEditProjectHandler(),
			"importMembers":            NewImportMembersHandler(),
			"listProjectInvitedGroups": NewListProjectInvitedGroupsHandler(),
			"listShareableGroups":      NewListShareableGroupsHandler(),
			"listTransferableGroups":   NewListTransferableGroupsHandler(),
			"restoreProject":           NewRestoreProjectHandler(),
			// Auto-generated actions
			"archiveProject":               NewArchiveProjectHandler(),
			"deleteProject":                NewDeleteProjectHandler(),
			"deleteSharedProjectFromGroup": NewDeleteSharedProjectFromGroupHandler(),
			"getProject":                   NewGetProjectHandler(),
			"getProjectLanguages":          NewGetProjectLanguagesHandler(),
			"listProjects":                 NewListProjectsHandler(),
			"listProjectsGroups":           NewListProjectsGroupsHandler(),
			"listProjectsUsers":            NewListProjectsUsersHandler(),
			"listUserContributedProjects":  NewListUserContributedProjectsHandler(),
			"listUserProjects":             NewListUserProjectsHandler(),
			"shareProjectWithGroup":        NewShareProjectWithGroupHandler(),
			"starProject":                  NewStarProjectHandler(),
			"startHousekeepingProject":     NewStartHousekeepingProjectHandler(),
			"transferProject":              NewTransferProjectHandler(),
			"unarchiveProject":             NewUnarchiveProjectHandler(),
			"unstarProject":                NewUnstarProjectHandler(),
		},
	}
}

func (h *GitlabProjectsBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}

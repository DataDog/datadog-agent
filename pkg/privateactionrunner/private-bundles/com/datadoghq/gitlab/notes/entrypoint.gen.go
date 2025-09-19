package com_datadoghq_gitlab_notes

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabNotesBundle struct {
	actions map[string]types.Action
}

func NewGitlabNotes() types.Bundle {
	return &GitlabNotesBundle{
		actions: map[string]types.Action{
			// Auto-generated actions
			"createDraftNote":        NewCreateDraftNoteHandler(),
			"createEpicNote":         NewCreateEpicNoteHandler(),
			"createIssueNote":        NewCreateIssueNoteHandler(),
			"createMergeRequestNote": NewCreateMergeRequestNoteHandler(),
			"createSnippetNote":      NewCreateSnippetNoteHandler(),
			"deleteDraftNote":        NewDeleteDraftNoteHandler(),
			"deleteEpicNote":         NewDeleteEpicNoteHandler(),
			"deleteIssueNote":        NewDeleteIssueNoteHandler(),
			"deleteMergeRequestNote": NewDeleteMergeRequestNoteHandler(),
			"deleteSnippetNote":      NewDeleteSnippetNoteHandler(),
			"getDraftNote":           NewGetDraftNoteHandler(),
			"getEpicNote":            NewGetEpicNoteHandler(),
			"getIssueNote":           NewGetIssueNoteHandler(),
			"getMergeRequestNote":    NewGetMergeRequestNoteHandler(),
			"getSnippetNote":         NewGetSnippetNoteHandler(),
			"listDraftNotes":         NewListDraftNotesHandler(),
			"listEpicNotes":          NewListEpicNotesHandler(),
			"listIssueNotes":         NewListIssueNotesHandler(),
			"listMergeRequestNotes":  NewListMergeRequestNotesHandler(),
			"listSnippetNotes":       NewListSnippetNotesHandler(),
			"publishAllDraftNotes":   NewPublishAllDraftNotesHandler(),
			"publishDraftNote":       NewPublishDraftNoteHandler(),
			"updateDraftNote":        NewUpdateDraftNoteHandler(),
			"updateEpicNote":         NewUpdateEpicNoteHandler(),
			"updateIssueNote":        NewUpdateIssueNoteHandler(),
			"updateMergeRequestNote": NewUpdateMergeRequestNoteHandler(),
			"updateSnippetNote":      NewUpdateSnippetNoteHandler(),
		},
	}
}

func (h *GitlabNotesBundle) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}

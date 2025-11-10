"""
GitLab API:
https://docs.gitlab.com/ee/api/merge_requests.html
https://docs.gitlab.com/ee/api/merge_request_approvals.html
"""
from typing import Any, cast, Dict, Optional, TYPE_CHECKING, Union

import requests

import gitlab
from gitlab import cli
from gitlab import exceptions as exc
from gitlab import types
from gitlab.base import RESTManager, RESTObject, RESTObjectList
from gitlab.mixins import (
    CRUDMixin,
    ListMixin,
    ObjectDeleteMixin,
    ParticipantsMixin,
    RetrieveMixin,
    SaveMixin,
    SubscribableMixin,
    TimeTrackingMixin,
    TodoMixin,
)
from gitlab.types import RequiredOptional

from .award_emojis import ProjectMergeRequestAwardEmojiManager  # noqa: F401
from .commits import ProjectCommit, ProjectCommitManager
from .discussions import ProjectMergeRequestDiscussionManager  # noqa: F401
from .draft_notes import ProjectMergeRequestDraftNoteManager
from .events import (  # noqa: F401
    ProjectMergeRequestResourceLabelEventManager,
    ProjectMergeRequestResourceMilestoneEventManager,
    ProjectMergeRequestResourceStateEventManager,
)
from .issues import ProjectIssue, ProjectIssueManager
from .merge_request_approvals import (  # noqa: F401
    ProjectMergeRequestApprovalManager,
    ProjectMergeRequestApprovalRuleManager,
    ProjectMergeRequestApprovalStateManager,
)
from .notes import ProjectMergeRequestNoteManager  # noqa: F401
from .pipelines import ProjectMergeRequestPipelineManager  # noqa: F401
from .reviewers import ProjectMergeRequestReviewerDetailManager

__all__ = [
    "MergeRequest",
    "MergeRequestManager",
    "GroupMergeRequest",
    "GroupMergeRequestManager",
    "ProjectMergeRequest",
    "ProjectMergeRequestManager",
    "ProjectDeploymentMergeRequest",
    "ProjectDeploymentMergeRequestManager",
    "ProjectMergeRequestDiff",
    "ProjectMergeRequestDiffManager",
]


class MergeRequest(RESTObject):
    pass


class MergeRequestManager(ListMixin, RESTManager):
    _path = "/merge_requests"
    _obj_cls = MergeRequest
    _list_filters = (
        "state",
        "order_by",
        "sort",
        "milestone",
        "view",
        "labels",
        "with_labels_details",
        "with_merge_status_recheck",
        "created_after",
        "created_before",
        "updated_after",
        "updated_before",
        "scope",
        "author_id",
        "author_username",
        "assignee_id",
        "approver_ids",
        "approved_by_ids",
        "reviewer_id",
        "reviewer_username",
        "my_reaction_emoji",
        "source_branch",
        "target_branch",
        "search",
        "in",
        "wip",
        "not",
        "environment",
        "deployed_before",
        "deployed_after",
    )
    _types = {
        "approver_ids": types.ArrayAttribute,
        "approved_by_ids": types.ArrayAttribute,
        "in": types.CommaSeparatedListAttribute,
        "labels": types.CommaSeparatedListAttribute,
    }


class GroupMergeRequest(RESTObject):
    pass


class GroupMergeRequestManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/merge_requests"
    _obj_cls = GroupMergeRequest
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = (
        "state",
        "order_by",
        "sort",
        "milestone",
        "view",
        "labels",
        "created_after",
        "created_before",
        "updated_after",
        "updated_before",
        "scope",
        "author_id",
        "assignee_id",
        "approver_ids",
        "approved_by_ids",
        "my_reaction_emoji",
        "source_branch",
        "target_branch",
        "search",
        "wip",
    )
    _types = {
        "approver_ids": types.ArrayAttribute,
        "approved_by_ids": types.ArrayAttribute,
        "labels": types.CommaSeparatedListAttribute,
    }


class ProjectMergeRequest(
    SubscribableMixin,
    TodoMixin,
    TimeTrackingMixin,
    ParticipantsMixin,
    SaveMixin,
    ObjectDeleteMixin,
    RESTObject,
):
    _id_attr = "iid"

    approval_rules: ProjectMergeRequestApprovalRuleManager
    approval_state: ProjectMergeRequestApprovalStateManager
    approvals: ProjectMergeRequestApprovalManager
    awardemojis: ProjectMergeRequestAwardEmojiManager
    diffs: "ProjectMergeRequestDiffManager"
    discussions: ProjectMergeRequestDiscussionManager
    draft_notes: ProjectMergeRequestDraftNoteManager
    notes: ProjectMergeRequestNoteManager
    pipelines: ProjectMergeRequestPipelineManager
    resourcelabelevents: ProjectMergeRequestResourceLabelEventManager
    resourcemilestoneevents: ProjectMergeRequestResourceMilestoneEventManager
    resourcestateevents: ProjectMergeRequestResourceStateEventManager
    reviewer_details: ProjectMergeRequestReviewerDetailManager

    @cli.register_custom_action("ProjectMergeRequest")
    @exc.on_http_error(exc.GitlabMROnBuildSuccessError)
    def cancel_merge_when_pipeline_succeeds(self, **kwargs: Any) -> Dict[str, str]:
        """Cancel merge when the pipeline succeeds.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabMROnBuildSuccessError: If the server could not handle the
                request

        Returns:
            dict of the parsed json returned by the server
        """

        path = (
            f"{self.manager.path}/{self.encoded_id}/cancel_merge_when_pipeline_succeeds"
        )
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        # 2022-10-30: The docs at
        # https://docs.gitlab.com/ee/api/merge_requests.html#cancel-merge-when-pipeline-succeeds
        # are incorrect in that the return value is actually just:
        #   {'status': 'success'}  for a successful cancel.
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        return server_data

    @cli.register_custom_action("ProjectMergeRequest")
    @exc.on_http_error(exc.GitlabListError)
    def closes_issues(self, **kwargs: Any) -> RESTObjectList:
        """List issues that will close on merge."

        Args:
            all: If True, return all the items, without pagination
            per_page: Number of items to retrieve per request
            page: ID of the page to return (starts with page 1)
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabListError: If the list could not be retrieved

        Returns:
            List of issues
        """
        path = f"{self.manager.path}/{self.encoded_id}/closes_issues"
        data_list = self.manager.gitlab.http_list(path, iterator=True, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(data_list, gitlab.GitlabList)
        manager = ProjectIssueManager(self.manager.gitlab, parent=self.manager._parent)
        return RESTObjectList(manager, ProjectIssue, data_list)

    @cli.register_custom_action("ProjectMergeRequest")
    @exc.on_http_error(exc.GitlabListError)
    def commits(self, **kwargs: Any) -> RESTObjectList:
        """List the merge request commits.

        Args:
            all: If True, return all the items, without pagination
            per_page: Number of items to retrieve per request
            page: ID of the page to return (starts with page 1)
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabListError: If the list could not be retrieved

        Returns:
            The list of commits
        """

        path = f"{self.manager.path}/{self.encoded_id}/commits"
        data_list = self.manager.gitlab.http_list(path, iterator=True, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(data_list, gitlab.GitlabList)
        manager = ProjectCommitManager(self.manager.gitlab, parent=self.manager._parent)
        return RESTObjectList(manager, ProjectCommit, data_list)

    @cli.register_custom_action("ProjectMergeRequest", optional=("access_raw_diffs",))
    @exc.on_http_error(exc.GitlabListError)
    def changes(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """List the merge request changes.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabListError: If the list could not be retrieved

        Returns:
            List of changes
        """
        path = f"{self.manager.path}/{self.encoded_id}/changes"
        return self.manager.gitlab.http_get(path, **kwargs)

    @cli.register_custom_action("ProjectMergeRequest", (), ("sha",))
    @exc.on_http_error(exc.GitlabMRApprovalError)
    def approve(self, sha: Optional[str] = None, **kwargs: Any) -> Dict[str, Any]:
        """Approve the merge request.

        Args:
            sha: Head SHA of MR
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabMRApprovalError: If the approval failed

        Returns:
           A dict containing the result.

        https://docs.gitlab.com/ee/api/merge_request_approvals.html#approve-merge-request
        """
        path = f"{self.manager.path}/{self.encoded_id}/approve"
        data = {}
        if sha:
            data["sha"] = sha

        server_data = self.manager.gitlab.http_post(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)
        return server_data

    @cli.register_custom_action("ProjectMergeRequest")
    @exc.on_http_error(exc.GitlabMRApprovalError)
    def unapprove(self, **kwargs: Any) -> None:
        """Unapprove the merge request.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabMRApprovalError: If the unapproval failed

        https://docs.gitlab.com/ee/api/merge_request_approvals.html#unapprove-merge-request
        """
        path = f"{self.manager.path}/{self.encoded_id}/unapprove"
        data: Dict[str, Any] = {}

        server_data = self.manager.gitlab.http_post(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("ProjectMergeRequest")
    @exc.on_http_error(exc.GitlabMRRebaseError)
    def rebase(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Attempt to rebase the source branch onto the target branch

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabMRRebaseError: If rebasing failed
        """
        path = f"{self.manager.path}/{self.encoded_id}/rebase"
        data: Dict[str, Any] = {}
        return self.manager.gitlab.http_put(path, post_data=data, **kwargs)

    @cli.register_custom_action("ProjectMergeRequest")
    @exc.on_http_error(exc.GitlabMRResetApprovalError)
    def reset_approvals(
        self, **kwargs: Any
    ) -> Union[Dict[str, Any], requests.Response]:
        """Clear all approvals of the merge request.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabMRResetApprovalError: If reset approval failed
        """
        path = f"{self.manager.path}/{self.encoded_id}/reset_approvals"
        data: Dict[str, Any] = {}
        return self.manager.gitlab.http_put(path, post_data=data, **kwargs)

    @cli.register_custom_action("ProjectMergeRequest")
    @exc.on_http_error(exc.GitlabGetError)
    def merge_ref(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Attempt to merge changes between source and target branches into
            `refs/merge-requests/:iid/merge`.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabGetError: If cannot be merged
        """
        path = f"{self.manager.path}/{self.encoded_id}/merge_ref"
        return self.manager.gitlab.http_get(path, **kwargs)

    @cli.register_custom_action(
        "ProjectMergeRequest",
        (),
        (
            "merge_commit_message",
            "should_remove_source_branch",
            "merge_when_pipeline_succeeds",
        ),
    )
    @exc.on_http_error(exc.GitlabMRClosedError)
    def merge(
        self,
        merge_commit_message: Optional[str] = None,
        should_remove_source_branch: Optional[bool] = None,
        merge_when_pipeline_succeeds: Optional[bool] = None,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Accept the merge request.

        Args:
            merge_commit_message: Commit message
            should_remove_source_branch: If True, removes the source
                                                branch
            merge_when_pipeline_succeeds: Wait for the build to succeed,
                                                 then merge
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabMRClosedError: If the merge failed
        """
        path = f"{self.manager.path}/{self.encoded_id}/merge"
        data: Dict[str, Any] = {}
        if merge_commit_message:
            data["merge_commit_message"] = merge_commit_message
        if should_remove_source_branch is not None:
            data["should_remove_source_branch"] = should_remove_source_branch
        if merge_when_pipeline_succeeds is not None:
            data["merge_when_pipeline_succeeds"] = merge_when_pipeline_succeeds

        server_data = self.manager.gitlab.http_put(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)
        return server_data


class ProjectMergeRequestManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests"
    _obj_cls = ProjectMergeRequest
    _from_parent_attrs = {"project_id": "id"}
    _optional_get_attrs = (
        "render_html",
        "include_diverged_commits_count",
        "include_rebase_in_progress",
    )
    _create_attrs = RequiredOptional(
        required=("source_branch", "target_branch", "title"),
        optional=(
            "allow_collaboration",
            "allow_maintainer_to_push",
            "approvals_before_merge",
            "assignee_id",
            "assignee_ids",
            "description",
            "labels",
            "milestone_id",
            "remove_source_branch",
            "reviewer_ids",
            "squash",
            "target_project_id",
        ),
    )
    _update_attrs = RequiredOptional(
        optional=(
            "target_branch",
            "assignee_id",
            "title",
            "description",
            "state_event",
            "labels",
            "milestone_id",
            "remove_source_branch",
            "discussion_locked",
            "allow_maintainer_to_push",
            "squash",
            "reviewer_ids",
        ),
    )
    _list_filters = (
        "state",
        "order_by",
        "sort",
        "milestone",
        "view",
        "labels",
        "created_after",
        "created_before",
        "updated_after",
        "updated_before",
        "scope",
        "iids",
        "author_id",
        "assignee_id",
        "approver_ids",
        "approved_by_ids",
        "my_reaction_emoji",
        "source_branch",
        "target_branch",
        "search",
        "wip",
    )
    _types = {
        "approver_ids": types.ArrayAttribute,
        "approved_by_ids": types.ArrayAttribute,
        "iids": types.ArrayAttribute,
        "labels": types.CommaSeparatedListAttribute,
    }

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequest:
        return cast(ProjectMergeRequest, super().get(id=id, lazy=lazy, **kwargs))


class ProjectDeploymentMergeRequest(MergeRequest):
    pass


class ProjectDeploymentMergeRequestManager(MergeRequestManager):
    _path = "/projects/{project_id}/deployments/{deployment_id}/merge_requests"
    _obj_cls = ProjectDeploymentMergeRequest
    _from_parent_attrs = {"deployment_id": "id", "project_id": "project_id"}


class ProjectMergeRequestDiff(RESTObject):
    pass


class ProjectMergeRequestDiffManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/versions"
    _obj_cls = ProjectMergeRequestDiff
    _from_parent_attrs = {"project_id": "project_id", "mr_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestDiff:
        return cast(ProjectMergeRequestDiff, super().get(id=id, lazy=lazy, **kwargs))

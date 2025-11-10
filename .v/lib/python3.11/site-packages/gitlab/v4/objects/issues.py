from typing import Any, cast, Dict, Optional, Tuple, TYPE_CHECKING, Union

from gitlab import cli
from gitlab import exceptions as exc
from gitlab import types
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    CRUDMixin,
    DeleteMixin,
    ListMixin,
    ObjectDeleteMixin,
    ParticipantsMixin,
    RetrieveMixin,
    SaveMixin,
    SubscribableMixin,
    TimeTrackingMixin,
    TodoMixin,
    UserAgentDetailMixin,
)
from gitlab.types import RequiredOptional

from .award_emojis import ProjectIssueAwardEmojiManager  # noqa: F401
from .discussions import ProjectIssueDiscussionManager  # noqa: F401
from .events import (  # noqa: F401
    ProjectIssueResourceIterationEventManager,
    ProjectIssueResourceLabelEventManager,
    ProjectIssueResourceMilestoneEventManager,
    ProjectIssueResourceStateEventManager,
    ProjectIssueResourceWeightEventManager,
)
from .notes import ProjectIssueNoteManager  # noqa: F401

__all__ = [
    "Issue",
    "IssueManager",
    "GroupIssue",
    "GroupIssueManager",
    "ProjectIssue",
    "ProjectIssueManager",
    "ProjectIssueLink",
    "ProjectIssueLinkManager",
]


class Issue(RESTObject):
    _url = "/issues"
    _repr_attr = "title"


class IssueManager(RetrieveMixin, RESTManager):
    _path = "/issues"
    _obj_cls = Issue
    _list_filters = (
        "state",
        "labels",
        "milestone",
        "scope",
        "author_id",
        "iteration_id",
        "assignee_id",
        "my_reaction_emoji",
        "iids",
        "order_by",
        "sort",
        "search",
        "created_after",
        "created_before",
        "updated_after",
        "updated_before",
    )
    _types = {"iids": types.ArrayAttribute, "labels": types.CommaSeparatedListAttribute}

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> Issue:
        return cast(Issue, super().get(id=id, lazy=lazy, **kwargs))


class GroupIssue(RESTObject):
    pass


class GroupIssueManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/issues"
    _obj_cls = GroupIssue
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = (
        "state",
        "labels",
        "milestone",
        "order_by",
        "sort",
        "iids",
        "author_id",
        "iteration_id",
        "assignee_id",
        "my_reaction_emoji",
        "search",
        "created_after",
        "created_before",
        "updated_after",
        "updated_before",
    )
    _types = {"iids": types.ArrayAttribute, "labels": types.CommaSeparatedListAttribute}


class ProjectIssue(
    UserAgentDetailMixin,
    SubscribableMixin,
    TodoMixin,
    TimeTrackingMixin,
    ParticipantsMixin,
    SaveMixin,
    ObjectDeleteMixin,
    RESTObject,
):
    _repr_attr = "title"
    _id_attr = "iid"

    awardemojis: ProjectIssueAwardEmojiManager
    discussions: ProjectIssueDiscussionManager
    links: "ProjectIssueLinkManager"
    notes: ProjectIssueNoteManager
    resourcelabelevents: ProjectIssueResourceLabelEventManager
    resourcemilestoneevents: ProjectIssueResourceMilestoneEventManager
    resourcestateevents: ProjectIssueResourceStateEventManager
    resource_iteration_events: ProjectIssueResourceIterationEventManager
    resource_weight_events: ProjectIssueResourceWeightEventManager

    @cli.register_custom_action("ProjectIssue", ("to_project_id",))
    @exc.on_http_error(exc.GitlabUpdateError)
    def move(self, to_project_id: int, **kwargs: Any) -> None:
        """Move the issue to another project.

        Args:
            to_project_id: ID of the target project
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUpdateError: If the issue could not be moved
        """
        path = f"{self.manager.path}/{self.encoded_id}/move"
        data = {"to_project_id": to_project_id}
        server_data = self.manager.gitlab.http_post(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("ProjectIssue", ("move_after_id", "move_before_id"))
    @exc.on_http_error(exc.GitlabUpdateError)
    def reorder(
        self,
        move_after_id: Optional[int] = None,
        move_before_id: Optional[int] = None,
        **kwargs: Any,
    ) -> None:
        """Reorder an issue on a board.

        Args:
            move_after_id: ID of an issue that should be placed after this issue
            move_before_id: ID of an issue that should be placed before this issue
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabUpdateError: If the issue could not be reordered
        """
        path = f"{self.manager.path}/{self.encoded_id}/reorder"
        data: Dict[str, Any] = {}

        if move_after_id is not None:
            data["move_after_id"] = move_after_id
        if move_before_id is not None:
            data["move_before_id"] = move_before_id

        server_data = self.manager.gitlab.http_put(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("ProjectIssue")
    @exc.on_http_error(exc.GitlabGetError)
    def related_merge_requests(self, **kwargs: Any) -> Dict[str, Any]:
        """List merge requests related to the issue.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetErrot: If the merge requests could not be retrieved

        Returns:
            The list of merge requests.
        """
        path = f"{self.manager.path}/{self.encoded_id}/related_merge_requests"
        result = self.manager.gitlab.http_get(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, dict)
        return result

    @cli.register_custom_action("ProjectIssue")
    @exc.on_http_error(exc.GitlabGetError)
    def closed_by(self, **kwargs: Any) -> Dict[str, Any]:
        """List merge requests that will close the issue when merged.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetErrot: If the merge requests could not be retrieved

        Returns:
            The list of merge requests.
        """
        path = f"{self.manager.path}/{self.encoded_id}/closed_by"
        result = self.manager.gitlab.http_get(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, dict)
        return result


class ProjectIssueManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/issues"
    _obj_cls = ProjectIssue
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = (
        "iids",
        "state",
        "labels",
        "milestone",
        "scope",
        "author_id",
        "iteration_id",
        "assignee_id",
        "my_reaction_emoji",
        "order_by",
        "sort",
        "search",
        "created_after",
        "created_before",
        "updated_after",
        "updated_before",
    )
    _create_attrs = RequiredOptional(
        required=("title",),
        optional=(
            "description",
            "confidential",
            "assignee_ids",
            "assignee_id",
            "milestone_id",
            "labels",
            "created_at",
            "due_date",
            "merge_request_to_resolve_discussions_of",
            "discussion_to_resolve",
        ),
    )
    _update_attrs = RequiredOptional(
        optional=(
            "title",
            "description",
            "confidential",
            "assignee_ids",
            "assignee_id",
            "milestone_id",
            "labels",
            "state_event",
            "updated_at",
            "due_date",
            "discussion_locked",
        ),
    )
    _types = {"iids": types.ArrayAttribute, "labels": types.CommaSeparatedListAttribute}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssue:
        return cast(ProjectIssue, super().get(id=id, lazy=lazy, **kwargs))


class ProjectIssueLink(ObjectDeleteMixin, RESTObject):
    _id_attr = "issue_link_id"


class ProjectIssueLinkManager(ListMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/links"
    _obj_cls = ProjectIssueLink
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}
    _create_attrs = RequiredOptional(required=("target_project_id", "target_issue_iid"))

    @exc.on_http_error(exc.GitlabCreateError)
    # NOTE(jlvillal): Signature doesn't match CreateMixin.create() so ignore
    # type error
    def create(  # type: ignore
        self, data: Dict[str, Any], **kwargs: Any
    ) -> Tuple[RESTObject, RESTObject]:
        """Create a new object.

        Args:
            data: parameters to send to the server to create the
                         resource
            **kwargs: Extra options to send to the server (e.g. sudo)

        Returns:
            The source and target issues

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server cannot perform the request
        """
        self._create_attrs.validate_attrs(data=data)
        if TYPE_CHECKING:
            assert self.path is not None
        server_data = self.gitlab.http_post(self.path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
            assert self._parent is not None
        source_issue = ProjectIssue(self._parent.manager, server_data["source_issue"])
        target_issue = ProjectIssue(self._parent.manager, server_data["target_issue"])
        return source_issue, target_issue

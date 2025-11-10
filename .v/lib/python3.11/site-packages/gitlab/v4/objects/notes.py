from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    CRUDMixin,
    DeleteMixin,
    GetMixin,
    ObjectDeleteMixin,
    RetrieveMixin,
    SaveMixin,
    UpdateMixin,
)
from gitlab.types import RequiredOptional

from .award_emojis import (  # noqa: F401
    GroupEpicNoteAwardEmojiManager,
    ProjectIssueNoteAwardEmojiManager,
    ProjectMergeRequestNoteAwardEmojiManager,
    ProjectSnippetNoteAwardEmojiManager,
)

__all__ = [
    "GroupEpicNote",
    "GroupEpicNoteManager",
    "GroupEpicDiscussionNote",
    "GroupEpicDiscussionNoteManager",
    "ProjectNote",
    "ProjectNoteManager",
    "ProjectCommitDiscussionNote",
    "ProjectCommitDiscussionNoteManager",
    "ProjectIssueNote",
    "ProjectIssueNoteManager",
    "ProjectIssueDiscussionNote",
    "ProjectIssueDiscussionNoteManager",
    "ProjectMergeRequestNote",
    "ProjectMergeRequestNoteManager",
    "ProjectMergeRequestDiscussionNote",
    "ProjectMergeRequestDiscussionNoteManager",
    "ProjectSnippetNote",
    "ProjectSnippetNoteManager",
    "ProjectSnippetDiscussionNote",
    "ProjectSnippetDiscussionNoteManager",
]


class GroupEpicNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    awardemojis: GroupEpicNoteAwardEmojiManager


class GroupEpicNoteManager(CRUDMixin, RESTManager):
    _path = "/groups/{group_id}/epics/{epic_id}/notes"
    _obj_cls = GroupEpicNote
    _from_parent_attrs = {"group_id": "group_id", "epic_id": "id"}
    _create_attrs = RequiredOptional(required=("body",), optional=("created_at",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupEpicNote:
        return cast(GroupEpicNote, super().get(id=id, lazy=lazy, **kwargs))


class GroupEpicDiscussionNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class GroupEpicDiscussionNoteManager(
    GetMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = "/groups/{group_id}/epics/{epic_id}/discussions/{discussion_id}/notes"
    _obj_cls = GroupEpicDiscussionNote
    _from_parent_attrs = {
        "group_id": "group_id",
        "epic_id": "epic_id",
        "discussion_id": "id",
    }
    _create_attrs = RequiredOptional(required=("body",), optional=("created_at",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupEpicDiscussionNote:
        return cast(GroupEpicDiscussionNote, super().get(id=id, lazy=lazy, **kwargs))


class ProjectNote(RESTObject):
    pass


class ProjectNoteManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/notes"
    _obj_cls = ProjectNote
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectNote:
        return cast(ProjectNote, super().get(id=id, lazy=lazy, **kwargs))


class ProjectCommitDiscussionNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectCommitDiscussionNoteManager(
    GetMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = (
        "/projects/{project_id}/repository/commits/{commit_id}/"
        "discussions/{discussion_id}/notes"
    )
    _obj_cls = ProjectCommitDiscussionNote
    _from_parent_attrs = {
        "project_id": "project_id",
        "commit_id": "commit_id",
        "discussion_id": "id",
    }
    _create_attrs = RequiredOptional(
        required=("body",), optional=("created_at", "position")
    )
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectCommitDiscussionNote:
        return cast(
            ProjectCommitDiscussionNote, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectIssueNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    awardemojis: ProjectIssueNoteAwardEmojiManager


class ProjectIssueNoteManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/notes"
    _obj_cls = ProjectIssueNote
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}
    _create_attrs = RequiredOptional(required=("body",), optional=("created_at",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueNote:
        return cast(ProjectIssueNote, super().get(id=id, lazy=lazy, **kwargs))


class ProjectIssueDiscussionNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectIssueDiscussionNoteManager(
    GetMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = (
        "/projects/{project_id}/issues/{issue_iid}/discussions/{discussion_id}/notes"
    )
    _obj_cls = ProjectIssueDiscussionNote
    _from_parent_attrs = {
        "project_id": "project_id",
        "issue_iid": "issue_iid",
        "discussion_id": "id",
    }
    _create_attrs = RequiredOptional(required=("body",), optional=("created_at",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueDiscussionNote:
        return cast(ProjectIssueDiscussionNote, super().get(id=id, lazy=lazy, **kwargs))


class ProjectMergeRequestNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    awardemojis: ProjectMergeRequestNoteAwardEmojiManager


class ProjectMergeRequestNoteManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/notes"
    _obj_cls = ProjectMergeRequestNote
    _from_parent_attrs = {"project_id": "project_id", "mr_iid": "iid"}
    _create_attrs = RequiredOptional(required=("body",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestNote:
        return cast(ProjectMergeRequestNote, super().get(id=id, lazy=lazy, **kwargs))


class ProjectMergeRequestDiscussionNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectMergeRequestDiscussionNoteManager(
    GetMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = (
        "/projects/{project_id}/merge_requests/{mr_iid}/"
        "discussions/{discussion_id}/notes"
    )
    _obj_cls = ProjectMergeRequestDiscussionNote
    _from_parent_attrs = {
        "project_id": "project_id",
        "mr_iid": "mr_iid",
        "discussion_id": "id",
    }
    _create_attrs = RequiredOptional(required=("body",), optional=("created_at",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestDiscussionNote:
        return cast(
            ProjectMergeRequestDiscussionNote, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectSnippetNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    awardemojis: ProjectSnippetNoteAwardEmojiManager


class ProjectSnippetNoteManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/snippets/{snippet_id}/notes"
    _obj_cls = ProjectSnippetNote
    _from_parent_attrs = {"project_id": "project_id", "snippet_id": "id"}
    _create_attrs = RequiredOptional(required=("body",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectSnippetNote:
        return cast(ProjectSnippetNote, super().get(id=id, lazy=lazy, **kwargs))


class ProjectSnippetDiscussionNote(SaveMixin, ObjectDeleteMixin, RESTObject):
    pass


class ProjectSnippetDiscussionNoteManager(
    GetMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = (
        "/projects/{project_id}/snippets/{snippet_id}/"
        "discussions/{discussion_id}/notes"
    )
    _obj_cls = ProjectSnippetDiscussionNote
    _from_parent_attrs = {
        "project_id": "project_id",
        "snippet_id": "snippet_id",
        "discussion_id": "id",
    }
    _create_attrs = RequiredOptional(required=("body",), optional=("created_at",))
    _update_attrs = RequiredOptional(required=("body",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectSnippetDiscussionNote:
        return cast(
            ProjectSnippetDiscussionNote, super().get(id=id, lazy=lazy, **kwargs)
        )

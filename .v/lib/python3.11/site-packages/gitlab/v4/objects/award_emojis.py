from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import NoUpdateMixin, ObjectDeleteMixin
from gitlab.types import RequiredOptional

__all__ = [
    "GroupEpicAwardEmoji",
    "GroupEpicAwardEmojiManager",
    "GroupEpicNoteAwardEmoji",
    "GroupEpicNoteAwardEmojiManager",
    "ProjectIssueAwardEmoji",
    "ProjectIssueAwardEmojiManager",
    "ProjectIssueNoteAwardEmoji",
    "ProjectIssueNoteAwardEmojiManager",
    "ProjectMergeRequestAwardEmoji",
    "ProjectMergeRequestAwardEmojiManager",
    "ProjectMergeRequestNoteAwardEmoji",
    "ProjectMergeRequestNoteAwardEmojiManager",
    "ProjectSnippetAwardEmoji",
    "ProjectSnippetAwardEmojiManager",
    "ProjectSnippetNoteAwardEmoji",
    "ProjectSnippetNoteAwardEmojiManager",
]


class GroupEpicAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class GroupEpicAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/groups/{group_id}/epics/{epic_iid}/award_emoji"
    _obj_cls = GroupEpicAwardEmoji
    _from_parent_attrs = {"group_id": "group_id", "epic_iid": "iid"}
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupEpicAwardEmoji:
        return cast(GroupEpicAwardEmoji, super().get(id=id, lazy=lazy, **kwargs))


class GroupEpicNoteAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class GroupEpicNoteAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/groups/{group_id}/epics/{epic_iid}/notes/{note_id}/award_emoji"
    _obj_cls = GroupEpicNoteAwardEmoji
    _from_parent_attrs = {
        "group_id": "group_id",
        "epic_iid": "epic_iid",
        "note_id": "id",
    }
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupEpicNoteAwardEmoji:
        return cast(GroupEpicNoteAwardEmoji, super().get(id=id, lazy=lazy, **kwargs))


class ProjectIssueAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class ProjectIssueAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/award_emoji"
    _obj_cls = ProjectIssueAwardEmoji
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueAwardEmoji:
        return cast(ProjectIssueAwardEmoji, super().get(id=id, lazy=lazy, **kwargs))


class ProjectIssueNoteAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class ProjectIssueNoteAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/notes/{note_id}/award_emoji"
    _obj_cls = ProjectIssueNoteAwardEmoji
    _from_parent_attrs = {
        "project_id": "project_id",
        "issue_iid": "issue_iid",
        "note_id": "id",
    }
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueNoteAwardEmoji:
        return cast(ProjectIssueNoteAwardEmoji, super().get(id=id, lazy=lazy, **kwargs))


class ProjectMergeRequestAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class ProjectMergeRequestAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/award_emoji"
    _obj_cls = ProjectMergeRequestAwardEmoji
    _from_parent_attrs = {"project_id": "project_id", "mr_iid": "iid"}
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestAwardEmoji:
        return cast(
            ProjectMergeRequestAwardEmoji, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectMergeRequestNoteAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class ProjectMergeRequestNoteAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/notes/{note_id}/award_emoji"
    _obj_cls = ProjectMergeRequestNoteAwardEmoji
    _from_parent_attrs = {
        "project_id": "project_id",
        "mr_iid": "mr_iid",
        "note_id": "id",
    }
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestNoteAwardEmoji:
        return cast(
            ProjectMergeRequestNoteAwardEmoji, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectSnippetAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class ProjectSnippetAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/snippets/{snippet_id}/award_emoji"
    _obj_cls = ProjectSnippetAwardEmoji
    _from_parent_attrs = {"project_id": "project_id", "snippet_id": "id"}
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectSnippetAwardEmoji:
        return cast(ProjectSnippetAwardEmoji, super().get(id=id, lazy=lazy, **kwargs))


class ProjectSnippetNoteAwardEmoji(ObjectDeleteMixin, RESTObject):
    pass


class ProjectSnippetNoteAwardEmojiManager(NoUpdateMixin, RESTManager):
    _path = "/projects/{project_id}/snippets/{snippet_id}/notes/{note_id}/award_emoji"
    _obj_cls = ProjectSnippetNoteAwardEmoji
    _from_parent_attrs = {
        "project_id": "project_id",
        "snippet_id": "snippet_id",
        "note_id": "id",
    }
    _create_attrs = RequiredOptional(required=("name",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectSnippetNoteAwardEmoji:
        return cast(
            ProjectSnippetNoteAwardEmoji, super().get(id=id, lazy=lazy, **kwargs)
        )

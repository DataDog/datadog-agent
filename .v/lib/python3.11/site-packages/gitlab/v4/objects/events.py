from typing import Any, cast, Union

from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import ListMixin, RetrieveMixin

__all__ = [
    "Event",
    "EventManager",
    "GroupEpicResourceLabelEvent",
    "GroupEpicResourceLabelEventManager",
    "ProjectEvent",
    "ProjectEventManager",
    "ProjectIssueResourceLabelEvent",
    "ProjectIssueResourceLabelEventManager",
    "ProjectIssueResourceMilestoneEvent",
    "ProjectIssueResourceMilestoneEventManager",
    "ProjectIssueResourceStateEvent",
    "ProjectIssueResourceIterationEventManager",
    "ProjectIssueResourceWeightEventManager",
    "ProjectIssueResourceIterationEvent",
    "ProjectIssueResourceWeightEvent",
    "ProjectIssueResourceStateEventManager",
    "ProjectMergeRequestResourceLabelEvent",
    "ProjectMergeRequestResourceLabelEventManager",
    "ProjectMergeRequestResourceMilestoneEvent",
    "ProjectMergeRequestResourceMilestoneEventManager",
    "ProjectMergeRequestResourceStateEvent",
    "ProjectMergeRequestResourceStateEventManager",
    "UserEvent",
    "UserEventManager",
]


class Event(RESTObject):
    _id_attr = None
    _repr_attr = "target_title"


class EventManager(ListMixin, RESTManager):
    _path = "/events"
    _obj_cls = Event
    _list_filters = ("action", "target_type", "before", "after", "sort", "scope")


class GroupEpicResourceLabelEvent(RESTObject):
    pass


class GroupEpicResourceLabelEventManager(RetrieveMixin, RESTManager):
    _path = "/groups/{group_id}/epics/{epic_id}/resource_label_events"
    _obj_cls = GroupEpicResourceLabelEvent
    _from_parent_attrs = {"group_id": "group_id", "epic_id": "id"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> GroupEpicResourceLabelEvent:
        return cast(
            GroupEpicResourceLabelEvent, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectEvent(Event):
    pass


class ProjectEventManager(EventManager):
    _path = "/projects/{project_id}/events"
    _obj_cls = ProjectEvent
    _from_parent_attrs = {"project_id": "id"}


class ProjectIssueResourceLabelEvent(RESTObject):
    pass


class ProjectIssueResourceLabelEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/resource_label_events"
    _obj_cls = ProjectIssueResourceLabelEvent
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueResourceLabelEvent:
        return cast(
            ProjectIssueResourceLabelEvent, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectIssueResourceMilestoneEvent(RESTObject):
    pass


class ProjectIssueResourceMilestoneEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/resource_milestone_events"
    _obj_cls = ProjectIssueResourceMilestoneEvent
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueResourceMilestoneEvent:
        return cast(
            ProjectIssueResourceMilestoneEvent, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectIssueResourceStateEvent(RESTObject):
    pass


class ProjectIssueResourceStateEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/resource_state_events"
    _obj_cls = ProjectIssueResourceStateEvent
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueResourceStateEvent:
        return cast(
            ProjectIssueResourceStateEvent, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectIssueResourceIterationEvent(RESTObject):
    pass


class ProjectIssueResourceIterationEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/resource_iteration_events"
    _obj_cls = ProjectIssueResourceIterationEvent
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueResourceIterationEvent:
        return cast(
            ProjectIssueResourceIterationEvent, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectIssueResourceWeightEvent(RESTObject):
    pass


class ProjectIssueResourceWeightEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/issues/{issue_iid}/resource_weight_events"
    _obj_cls = ProjectIssueResourceWeightEvent
    _from_parent_attrs = {"project_id": "project_id", "issue_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIssueResourceWeightEvent:
        return cast(
            ProjectIssueResourceWeightEvent, super().get(id=id, lazy=lazy, **kwargs)
        )


class ProjectMergeRequestResourceLabelEvent(RESTObject):
    pass


class ProjectMergeRequestResourceLabelEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/resource_label_events"
    _obj_cls = ProjectMergeRequestResourceLabelEvent
    _from_parent_attrs = {"project_id": "project_id", "mr_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestResourceLabelEvent:
        return cast(
            ProjectMergeRequestResourceLabelEvent,
            super().get(id=id, lazy=lazy, **kwargs),
        )


class ProjectMergeRequestResourceMilestoneEvent(RESTObject):
    pass


class ProjectMergeRequestResourceMilestoneEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/resource_milestone_events"
    _obj_cls = ProjectMergeRequestResourceMilestoneEvent
    _from_parent_attrs = {"project_id": "project_id", "mr_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestResourceMilestoneEvent:
        return cast(
            ProjectMergeRequestResourceMilestoneEvent,
            super().get(id=id, lazy=lazy, **kwargs),
        )


class ProjectMergeRequestResourceStateEvent(RESTObject):
    pass


class ProjectMergeRequestResourceStateEventManager(RetrieveMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/resource_state_events"
    _obj_cls = ProjectMergeRequestResourceStateEvent
    _from_parent_attrs = {"project_id": "project_id", "mr_iid": "iid"}

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectMergeRequestResourceStateEvent:
        return cast(
            ProjectMergeRequestResourceStateEvent,
            super().get(id=id, lazy=lazy, **kwargs),
        )


class UserEvent(Event):
    pass


class UserEventManager(EventManager):
    _path = "/users/{user_id}/events"
    _obj_cls = UserEvent
    _from_parent_attrs = {"user_id": "id"}

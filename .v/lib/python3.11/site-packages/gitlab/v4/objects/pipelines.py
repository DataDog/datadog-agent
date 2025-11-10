from typing import Any, cast, Dict, Optional, TYPE_CHECKING, Union

import requests

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    CRUDMixin,
    DeleteMixin,
    GetWithoutIdMixin,
    ListMixin,
    ObjectDeleteMixin,
    RefreshMixin,
    RetrieveMixin,
    SaveMixin,
    UpdateMixin,
)
from gitlab.types import RequiredOptional

__all__ = [
    "ProjectMergeRequestPipeline",
    "ProjectMergeRequestPipelineManager",
    "ProjectPipeline",
    "ProjectPipelineManager",
    "ProjectPipelineJob",
    "ProjectPipelineJobManager",
    "ProjectPipelineBridge",
    "ProjectPipelineBridgeManager",
    "ProjectPipelineVariable",
    "ProjectPipelineVariableManager",
    "ProjectPipelineScheduleVariable",
    "ProjectPipelineScheduleVariableManager",
    "ProjectPipelineSchedulePipeline",
    "ProjectPipelineSchedulePipelineManager",
    "ProjectPipelineSchedule",
    "ProjectPipelineScheduleManager",
    "ProjectPipelineTestReport",
    "ProjectPipelineTestReportManager",
    "ProjectPipelineTestReportSummary",
    "ProjectPipelineTestReportSummaryManager",
]


class ProjectMergeRequestPipeline(RESTObject):
    pass


class ProjectMergeRequestPipelineManager(CreateMixin, ListMixin, RESTManager):
    _path = "/projects/{project_id}/merge_requests/{mr_iid}/pipelines"
    _obj_cls = ProjectMergeRequestPipeline
    _from_parent_attrs = {"project_id": "project_id", "mr_iid": "iid"}


class ProjectPipeline(RefreshMixin, ObjectDeleteMixin, RESTObject):
    bridges: "ProjectPipelineBridgeManager"
    jobs: "ProjectPipelineJobManager"
    test_report: "ProjectPipelineTestReportManager"
    test_report_summary: "ProjectPipelineTestReportSummaryManager"
    variables: "ProjectPipelineVariableManager"

    @cli.register_custom_action("ProjectPipeline")
    @exc.on_http_error(exc.GitlabPipelineCancelError)
    def cancel(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Cancel the job.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabPipelineCancelError: If the request failed
        """
        path = f"{self.manager.path}/{self.encoded_id}/cancel"
        return self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("ProjectPipeline")
    @exc.on_http_error(exc.GitlabPipelineRetryError)
    def retry(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Retry the job.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabPipelineRetryError: If the request failed
        """
        path = f"{self.manager.path}/{self.encoded_id}/retry"
        return self.manager.gitlab.http_post(path, **kwargs)


class ProjectPipelineManager(RetrieveMixin, CreateMixin, DeleteMixin, RESTManager):
    _path = "/projects/{project_id}/pipelines"
    _obj_cls = ProjectPipeline
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = (
        "scope",
        "status",
        "source",
        "ref",
        "sha",
        "yaml_errors",
        "name",
        "username",
        "order_by",
        "sort",
    )
    _create_attrs = RequiredOptional(required=("ref",))

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectPipeline:
        return cast(ProjectPipeline, super().get(id=id, lazy=lazy, **kwargs))

    def create(
        self, data: Optional[Dict[str, Any]] = None, **kwargs: Any
    ) -> ProjectPipeline:
        """Creates a new object.

        Args:
            data: Parameters to send to the server to create the
                         resource
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server cannot perform the request

        Returns:
            A new instance of the managed object class build with
                the data sent by the server
        """
        if TYPE_CHECKING:
            assert self.path is not None
        path = self.path[:-1]  # drop the 's'
        return cast(
            ProjectPipeline, CreateMixin.create(self, data, path=path, **kwargs)
        )


class ProjectPipelineJob(RESTObject):
    pass


class ProjectPipelineJobManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/pipelines/{pipeline_id}/jobs"
    _obj_cls = ProjectPipelineJob
    _from_parent_attrs = {"project_id": "project_id", "pipeline_id": "id"}
    _list_filters = ("scope", "include_retried")


class ProjectPipelineBridge(RESTObject):
    pass


class ProjectPipelineBridgeManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/pipelines/{pipeline_id}/bridges"
    _obj_cls = ProjectPipelineBridge
    _from_parent_attrs = {"project_id": "project_id", "pipeline_id": "id"}
    _list_filters = ("scope",)


class ProjectPipelineVariable(RESTObject):
    _id_attr = "key"


class ProjectPipelineVariableManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/pipelines/{pipeline_id}/variables"
    _obj_cls = ProjectPipelineVariable
    _from_parent_attrs = {"project_id": "project_id", "pipeline_id": "id"}


class ProjectPipelineScheduleVariable(SaveMixin, ObjectDeleteMixin, RESTObject):
    _id_attr = "key"


class ProjectPipelineScheduleVariableManager(
    CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = "/projects/{project_id}/pipeline_schedules/{pipeline_schedule_id}/variables"
    _obj_cls = ProjectPipelineScheduleVariable
    _from_parent_attrs = {"project_id": "project_id", "pipeline_schedule_id": "id"}
    _create_attrs = RequiredOptional(required=("key", "value"))
    _update_attrs = RequiredOptional(required=("key", "value"))


class ProjectPipelineSchedulePipeline(RESTObject):
    pass


class ProjectPipelineSchedulePipelineManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/pipeline_schedules/{pipeline_schedule_id}/pipelines"
    _obj_cls = ProjectPipelineSchedulePipeline
    _from_parent_attrs = {"project_id": "project_id", "pipeline_schedule_id": "id"}


class ProjectPipelineSchedule(SaveMixin, ObjectDeleteMixin, RESTObject):
    variables: ProjectPipelineScheduleVariableManager
    pipelines: ProjectPipelineSchedulePipelineManager

    @cli.register_custom_action("ProjectPipelineSchedule")
    @exc.on_http_error(exc.GitlabOwnershipError)
    def take_ownership(self, **kwargs: Any) -> None:
        """Update the owner of a pipeline schedule.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabOwnershipError: If the request failed
        """
        path = f"{self.manager.path}/{self.encoded_id}/take_ownership"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("ProjectPipelineSchedule")
    @exc.on_http_error(exc.GitlabPipelinePlayError)
    def play(self, **kwargs: Any) -> Dict[str, Any]:
        """Trigger a new scheduled pipeline, which runs immediately.
        The next scheduled run of this pipeline is not affected.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabPipelinePlayError: If the request failed
        """
        path = f"{self.manager.path}/{self.encoded_id}/play"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)
        return server_data


class ProjectPipelineScheduleManager(CRUDMixin, RESTManager):
    _path = "/projects/{project_id}/pipeline_schedules"
    _obj_cls = ProjectPipelineSchedule
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("description", "ref", "cron"), optional=("cron_timezone", "active")
    )
    _update_attrs = RequiredOptional(
        optional=("description", "ref", "cron", "cron_timezone", "active"),
    )

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectPipelineSchedule:
        return cast(ProjectPipelineSchedule, super().get(id=id, lazy=lazy, **kwargs))


class ProjectPipelineTestReport(RESTObject):
    _id_attr = None


class ProjectPipelineTestReportManager(GetWithoutIdMixin, RESTManager):
    _path = "/projects/{project_id}/pipelines/{pipeline_id}/test_report"
    _obj_cls = ProjectPipelineTestReport
    _from_parent_attrs = {"project_id": "project_id", "pipeline_id": "id"}

    def get(self, **kwargs: Any) -> ProjectPipelineTestReport:
        return cast(ProjectPipelineTestReport, super().get(**kwargs))


class ProjectPipelineTestReportSummary(RESTObject):
    _id_attr = None


class ProjectPipelineTestReportSummaryManager(GetWithoutIdMixin, RESTManager):
    _path = "/projects/{project_id}/pipelines/{pipeline_id}/test_report_summary"
    _obj_cls = ProjectPipelineTestReportSummary
    _from_parent_attrs = {"project_id": "project_id", "pipeline_id": "id"}

    def get(self, **kwargs: Any) -> ProjectPipelineTestReportSummary:
        return cast(ProjectPipelineTestReportSummary, super().get(**kwargs))

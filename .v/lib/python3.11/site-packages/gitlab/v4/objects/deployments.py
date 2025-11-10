"""
GitLab API:
https://docs.gitlab.com/ee/api/deployments.html
"""
from typing import Any, cast, Dict, Optional, TYPE_CHECKING, Union

from gitlab import cli
from gitlab import exceptions as exc
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import CreateMixin, RetrieveMixin, SaveMixin, UpdateMixin
from gitlab.types import RequiredOptional

from .merge_requests import ProjectDeploymentMergeRequestManager  # noqa: F401

__all__ = [
    "ProjectDeployment",
    "ProjectDeploymentManager",
]


class ProjectDeployment(SaveMixin, RESTObject):
    mergerequests: ProjectDeploymentMergeRequestManager

    @cli.register_custom_action(
        "ProjectDeployment",
        mandatory=("status",),
        optional=("comment", "represented_as"),
    )
    @exc.on_http_error(exc.GitlabDeploymentApprovalError)
    def approval(
        self,
        status: str,
        comment: Optional[str] = None,
        represented_as: Optional[str] = None,
        **kwargs: Any,
    ) -> Dict[str, Any]:
        """Approve or reject a blocked deployment.

        Args:
            status: Either "approved" or "rejected"
            comment: A comment to go with the approval
            represented_as: The name of the User/Group/Role to use for the
                            approval, when the user belongs to multiple
                            approval rules.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabMRApprovalError: If the approval failed

        Returns:
           A dict containing the result.

        https://docs.gitlab.com/ee/api/deployments.html#approve-or-reject-a-blocked-deployment
        """
        path = f"{self.manager.path}/{self.encoded_id}/approval"
        data = {"status": status}
        if comment is not None:
            data["comment"] = comment
        if represented_as is not None:
            data["represented_as"] = represented_as

        server_data = self.manager.gitlab.http_post(path, post_data=data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        return server_data


class ProjectDeploymentManager(RetrieveMixin, CreateMixin, UpdateMixin, RESTManager):
    _path = "/projects/{project_id}/deployments"
    _obj_cls = ProjectDeployment
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = (
        "order_by",
        "sort",
        "updated_after",
        "updated_before",
        "environment",
        "status",
    )
    _create_attrs = RequiredOptional(
        required=("sha", "ref", "tag", "status", "environment")
    )

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectDeployment:
        return cast(ProjectDeployment, super().get(id=id, lazy=lazy, **kwargs))

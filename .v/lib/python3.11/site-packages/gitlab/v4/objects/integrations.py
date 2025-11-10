"""
GitLab API:
https://docs.gitlab.com/ee/api/integrations.html
"""

from typing import Any, cast, List, Union

from gitlab import cli
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    DeleteMixin,
    GetMixin,
    ListMixin,
    ObjectDeleteMixin,
    SaveMixin,
    UpdateMixin,
)

__all__ = [
    "ProjectIntegration",
    "ProjectIntegrationManager",
    "ProjectService",
    "ProjectServiceManager",
]


class ProjectIntegration(SaveMixin, ObjectDeleteMixin, RESTObject):
    _id_attr = "slug"


class ProjectIntegrationManager(
    GetMixin, UpdateMixin, DeleteMixin, ListMixin, RESTManager
):
    _path = "/projects/{project_id}/integrations"
    _from_parent_attrs = {"project_id": "id"}
    _obj_cls = ProjectIntegration

    _service_attrs = {
        "asana": (("api_key",), ("restrict_to_branch", "push_events")),
        "assembla": (("token",), ("subdomain", "push_events")),
        "bamboo": (
            ("bamboo_url", "build_key", "username", "password"),
            ("push_events",),
        ),
        "bugzilla": (
            ("new_issue_url", "issues_url", "project_url"),
            ("description", "title", "push_events"),
        ),
        "buildkite": (
            ("token", "project_url"),
            ("enable_ssl_verification", "push_events"),
        ),
        "campfire": (("token",), ("subdomain", "room", "push_events")),
        "circuit": (
            ("webhook",),
            (
                "notify_only_broken_pipelines",
                "branches_to_be_notified",
                "push_events",
                "issues_events",
                "confidential_issues_events",
                "merge_requests_events",
                "tag_push_events",
                "note_events",
                "confidential_note_events",
                "pipeline_events",
                "wiki_page_events",
            ),
        ),
        "custom-issue-tracker": (
            ("new_issue_url", "issues_url", "project_url"),
            ("description", "title", "push_events"),
        ),
        "drone-ci": (
            ("token", "drone_url"),
            (
                "enable_ssl_verification",
                "push_events",
                "merge_requests_events",
                "tag_push_events",
            ),
        ),
        "emails-on-push": (
            ("recipients",),
            (
                "disable_diffs",
                "send_from_committer_email",
                "push_events",
                "tag_push_events",
                "branches_to_be_notified",
            ),
        ),
        "pipelines-email": (
            ("recipients",),
            (
                "add_pusher",
                "notify_only_broken_builds",
                "branches_to_be_notified",
                "notify_only_default_branch",
                "pipeline_events",
            ),
        ),
        "external-wiki": (("external_wiki_url",), ()),
        "flowdock": (("token",), ("push_events",)),
        "github": (("token", "repository_url"), ("static_context",)),
        "hangouts-chat": (
            ("webhook",),
            (
                "notify_only_broken_pipelines",
                "notify_only_default_branch",
                "branches_to_be_notified",
                "push_events",
                "issues_events",
                "confidential_issues_events",
                "merge_requests_events",
                "tag_push_events",
                "note_events",
                "confidential_note_events",
                "pipeline_events",
                "wiki_page_events",
            ),
        ),
        "hipchat": (
            ("token",),
            (
                "color",
                "notify",
                "room",
                "api_version",
                "server",
                "push_events",
                "issues_events",
                "confidential_issues_events",
                "merge_requests_events",
                "tag_push_events",
                "note_events",
                "confidential_note_events",
                "pipeline_events",
            ),
        ),
        "irker": (
            ("recipients",),
            (
                "default_irc_uri",
                "server_port",
                "server_host",
                "colorize_messages",
                "push_events",
            ),
        ),
        "jira": (
            (
                "url",
                "username",
                "password",
            ),
            (
                "api_url",
                "active",
                "jira_issue_transition_id",
                "commit_events",
                "merge_requests_events",
                "comment_on_event_enabled",
            ),
        ),
        "slack-slash-commands": (("token",), ()),
        "mattermost-slash-commands": (("token",), ("username",)),
        "packagist": (
            ("username", "token"),
            ("server", "push_events", "merge_requests_events", "tag_push_events"),
        ),
        "mattermost": (
            ("webhook",),
            (
                "username",
                "channel",
                "notify_only_broken_pipelines",
                "notify_only_default_branch",
                "branches_to_be_notified",
                "push_events",
                "issues_events",
                "confidential_issues_events",
                "merge_requests_events",
                "tag_push_events",
                "note_events",
                "confidential_note_events",
                "pipeline_events",
                "wiki_page_events",
                "push_channel",
                "issue_channel",
                "confidential_issue_channel",
                "merge_request_channel",
                "note_channel",
                "confidential_note_channel",
                "tag_push_channel",
                "pipeline_channel",
                "wiki_page_channel",
            ),
        ),
        "pivotaltracker": (("token",), ("restrict_to_branch", "push_events")),
        "prometheus": (("api_url",), ()),
        "pushover": (
            ("api_key", "user_key", "priority"),
            ("device", "sound", "push_events"),
        ),
        "redmine": (
            ("new_issue_url", "project_url", "issues_url"),
            ("description", "push_events"),
        ),
        "slack": (
            ("webhook",),
            (
                "username",
                "channel",
                "notify_only_broken_pipelines",
                "notify_only_default_branch",
                "branches_to_be_notified",
                "commit_events",
                "confidential_issue_channel",
                "confidential_issues_events",
                "confidential_note_channel",
                "confidential_note_events",
                "deployment_channel",
                "deployment_events",
                "issue_channel",
                "issues_events",
                "job_events",
                "merge_request_channel",
                "merge_requests_events",
                "note_channel",
                "note_events",
                "pipeline_channel",
                "pipeline_events",
                "push_channel",
                "push_events",
                "tag_push_channel",
                "tag_push_events",
                "wiki_page_channel",
                "wiki_page_events",
            ),
        ),
        "microsoft-teams": (
            ("webhook",),
            (
                "notify_only_broken_pipelines",
                "notify_only_default_branch",
                "branches_to_be_notified",
                "push_events",
                "issues_events",
                "confidential_issues_events",
                "merge_requests_events",
                "tag_push_events",
                "note_events",
                "confidential_note_events",
                "pipeline_events",
                "wiki_page_events",
            ),
        ),
        "teamcity": (
            ("teamcity_url", "build_type", "username", "password"),
            ("push_events",),
        ),
        "jenkins": (("jenkins_url", "project_name"), ("username", "password")),
        "mock-ci": (("mock_service_url",), ()),
        "youtrack": (("issues_url", "project_url"), ("description", "push_events")),
    }

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectIntegration:
        return cast(ProjectIntegration, super().get(id=id, lazy=lazy, **kwargs))

    @cli.register_custom_action(("ProjectIntegrationManager", "ProjectServiceManager"))
    def available(self) -> List[str]:
        """List the services known by python-gitlab.

        Returns:
            The list of service code names.
        """
        return list(self._service_attrs.keys())


class ProjectService(ProjectIntegration):
    pass


class ProjectServiceManager(ProjectIntegrationManager):
    _obj_cls = ProjectService

    def get(
        self, id: Union[str, int], lazy: bool = False, **kwargs: Any
    ) -> ProjectService:
        return cast(ProjectService, super().get(id=id, lazy=lazy, **kwargs))

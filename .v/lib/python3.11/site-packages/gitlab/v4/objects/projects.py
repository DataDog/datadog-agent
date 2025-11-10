"""
GitLab API:
https://docs.gitlab.com/ee/api/projects.html
"""
from typing import (
    Any,
    Callable,
    cast,
    Dict,
    Iterator,
    List,
    Optional,
    TYPE_CHECKING,
    Union,
)

import requests

from gitlab import cli, client
from gitlab import exceptions as exc
from gitlab import types, utils
from gitlab.base import RESTManager, RESTObject
from gitlab.mixins import (
    CreateMixin,
    CRUDMixin,
    DeleteMixin,
    GetWithoutIdMixin,
    ListMixin,
    ObjectDeleteMixin,
    RefreshMixin,
    SaveMixin,
    UpdateMixin,
    UploadMixin,
)
from gitlab.types import RequiredOptional

from .access_requests import ProjectAccessRequestManager  # noqa: F401
from .artifacts import ProjectArtifactManager  # noqa: F401
from .audit_events import ProjectAuditEventManager  # noqa: F401
from .badges import ProjectBadgeManager  # noqa: F401
from .boards import ProjectBoardManager  # noqa: F401
from .branches import ProjectBranchManager, ProjectProtectedBranchManager  # noqa: F401
from .ci_lint import ProjectCiLintManager  # noqa: F401
from .clusters import ProjectClusterManager  # noqa: F401
from .commits import ProjectCommitManager  # noqa: F401
from .container_registry import ProjectRegistryRepositoryManager  # noqa: F401
from .custom_attributes import ProjectCustomAttributeManager  # noqa: F401
from .deploy_keys import ProjectKeyManager  # noqa: F401
from .deploy_tokens import ProjectDeployTokenManager  # noqa: F401
from .deployments import ProjectDeploymentManager  # noqa: F401
from .environments import (  # noqa: F401
    ProjectEnvironmentManager,
    ProjectProtectedEnvironmentManager,
)
from .events import ProjectEventManager  # noqa: F401
from .export_import import ProjectExportManager, ProjectImportManager  # noqa: F401
from .files import ProjectFileManager  # noqa: F401
from .hooks import ProjectHookManager  # noqa: F401
from .integrations import ProjectIntegrationManager, ProjectServiceManager  # noqa: F401
from .invitations import ProjectInvitationManager  # noqa: F401
from .issues import ProjectIssueManager  # noqa: F401
from .iterations import ProjectIterationManager  # noqa: F401
from .job_token_scope import ProjectJobTokenScopeManager  # noqa: F401
from .jobs import ProjectJobManager  # noqa: F401
from .labels import ProjectLabelManager  # noqa: F401
from .members import ProjectMemberAllManager, ProjectMemberManager  # noqa: F401
from .merge_request_approvals import (  # noqa: F401
    ProjectApprovalManager,
    ProjectApprovalRuleManager,
)
from .merge_requests import ProjectMergeRequestManager  # noqa: F401
from .merge_trains import ProjectMergeTrainManager  # noqa: F401
from .milestones import ProjectMilestoneManager  # noqa: F401
from .notes import ProjectNoteManager  # noqa: F401
from .notification_settings import ProjectNotificationSettingsManager  # noqa: F401
from .packages import GenericPackageManager, ProjectPackageManager  # noqa: F401
from .pages import ProjectPagesDomainManager  # noqa: F401
from .pipelines import (  # noqa: F401
    ProjectPipeline,
    ProjectPipelineManager,
    ProjectPipelineScheduleManager,
)
from .project_access_tokens import ProjectAccessTokenManager  # noqa: F401
from .push_rules import ProjectPushRulesManager  # noqa: F401
from .releases import ProjectReleaseManager  # noqa: F401
from .repositories import RepositoryMixin
from .resource_groups import ProjectResourceGroupManager
from .runners import ProjectRunnerManager  # noqa: F401
from .secure_files import ProjectSecureFileManager  # noqa: F401
from .snippets import ProjectSnippetManager  # noqa: F401
from .statistics import (  # noqa: F401
    ProjectAdditionalStatisticsManager,
    ProjectIssuesStatisticsManager,
)
from .tags import ProjectProtectedTagManager, ProjectTagManager  # noqa: F401
from .triggers import ProjectTriggerManager  # noqa: F401
from .users import ProjectUserManager  # noqa: F401
from .variables import ProjectVariableManager  # noqa: F401
from .wikis import ProjectWikiManager  # noqa: F401

__all__ = [
    "GroupProject",
    "GroupProjectManager",
    "Project",
    "ProjectManager",
    "ProjectFork",
    "ProjectForkManager",
    "ProjectRemoteMirror",
    "ProjectRemoteMirrorManager",
    "ProjectStorage",
    "ProjectStorageManager",
    "SharedProject",
    "SharedProjectManager",
]


class GroupProject(RESTObject):
    pass


class GroupProjectManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/projects"
    _obj_cls = GroupProject
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = (
        "archived",
        "visibility",
        "order_by",
        "sort",
        "search",
        "simple",
        "owned",
        "starred",
        "with_custom_attributes",
        "include_subgroups",
        "with_issues_enabled",
        "with_merge_requests_enabled",
        "with_shared",
        "min_access_level",
        "with_security_reports",
    )


class ProjectGroup(RESTObject):
    pass


class ProjectGroupManager(ListMixin, RESTManager):
    _path = "/projects/{project_id}/groups"
    _obj_cls = ProjectGroup
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = (
        "search",
        "skip_groups",
        "with_shared",
        "shared_min_access_level",
        "shared_visible_only",
    )
    _types = {"skip_groups": types.ArrayAttribute}


class Project(
    RefreshMixin, SaveMixin, ObjectDeleteMixin, RepositoryMixin, UploadMixin, RESTObject
):
    _repr_attr = "path_with_namespace"
    _upload_path = "/projects/{id}/uploads"

    access_tokens: ProjectAccessTokenManager
    accessrequests: ProjectAccessRequestManager
    additionalstatistics: ProjectAdditionalStatisticsManager
    approvalrules: ProjectApprovalRuleManager
    approvals: ProjectApprovalManager
    artifacts: ProjectArtifactManager
    audit_events: ProjectAuditEventManager
    badges: ProjectBadgeManager
    boards: ProjectBoardManager
    branches: ProjectBranchManager
    ci_lint: ProjectCiLintManager
    clusters: ProjectClusterManager
    commits: ProjectCommitManager
    customattributes: ProjectCustomAttributeManager
    deployments: ProjectDeploymentManager
    deploytokens: ProjectDeployTokenManager
    environments: ProjectEnvironmentManager
    events: ProjectEventManager
    exports: ProjectExportManager
    files: ProjectFileManager
    forks: "ProjectForkManager"
    generic_packages: GenericPackageManager
    groups: ProjectGroupManager
    hooks: ProjectHookManager
    imports: ProjectImportManager
    integrations: ProjectIntegrationManager
    invitations: ProjectInvitationManager
    issues: ProjectIssueManager
    issues_statistics: ProjectIssuesStatisticsManager
    iterations: ProjectIterationManager
    jobs: ProjectJobManager
    job_token_scope: ProjectJobTokenScopeManager
    keys: ProjectKeyManager
    labels: ProjectLabelManager
    members: ProjectMemberManager
    members_all: ProjectMemberAllManager
    mergerequests: ProjectMergeRequestManager
    merge_trains: ProjectMergeTrainManager
    milestones: ProjectMilestoneManager
    notes: ProjectNoteManager
    notificationsettings: ProjectNotificationSettingsManager
    packages: ProjectPackageManager
    pagesdomains: ProjectPagesDomainManager
    pipelines: ProjectPipelineManager
    pipelineschedules: ProjectPipelineScheduleManager
    protected_environments: ProjectProtectedEnvironmentManager
    protectedbranches: ProjectProtectedBranchManager
    protectedtags: ProjectProtectedTagManager
    pushrules: ProjectPushRulesManager
    releases: ProjectReleaseManager
    resource_groups: ProjectResourceGroupManager
    remote_mirrors: "ProjectRemoteMirrorManager"
    repositories: ProjectRegistryRepositoryManager
    runners: ProjectRunnerManager
    secure_files: ProjectSecureFileManager
    services: ProjectServiceManager
    snippets: ProjectSnippetManager
    storage: "ProjectStorageManager"
    tags: ProjectTagManager
    triggers: ProjectTriggerManager
    users: ProjectUserManager
    variables: ProjectVariableManager
    wikis: ProjectWikiManager

    @cli.register_custom_action("Project", ("forked_from_id",))
    @exc.on_http_error(exc.GitlabCreateError)
    def create_fork_relation(self, forked_from_id: int, **kwargs: Any) -> None:
        """Create a forked from/to relation between existing projects.

        Args:
            forked_from_id: The ID of the project that was forked from
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the relation could not be created
        """
        path = f"/projects/{self.encoded_id}/fork/{forked_from_id}"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabDeleteError)
    def delete_fork_relation(self, **kwargs: Any) -> None:
        """Delete a forked relation between existing projects.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/fork"
        self.manager.gitlab.http_delete(path, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabGetError)
    def languages(self, **kwargs: Any) -> Union[Dict[str, Any], requests.Response]:
        """Get languages used in the project with percentage value.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/languages"
        return self.manager.gitlab.http_get(path, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabCreateError)
    def star(self, **kwargs: Any) -> None:
        """Star a project.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/star"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabDeleteError)
    def unstar(self, **kwargs: Any) -> None:
        """Unstar a project.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/unstar"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabCreateError)
    def archive(self, **kwargs: Any) -> None:
        """Archive a project.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/archive"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabDeleteError)
    def unarchive(self, **kwargs: Any) -> None:
        """Unarchive a project.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/unarchive"
        server_data = self.manager.gitlab.http_post(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(server_data, dict)
        self._update_attrs(server_data)

    @cli.register_custom_action(
        "Project", ("group_id", "group_access"), ("expires_at",)
    )
    @exc.on_http_error(exc.GitlabCreateError)
    def share(
        self,
        group_id: int,
        group_access: int,
        expires_at: Optional[str] = None,
        **kwargs: Any,
    ) -> None:
        """Share the project with a group.

        Args:
            group_id: ID of the group.
            group_access: Access level for the group.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/share"
        data = {
            "group_id": group_id,
            "group_access": group_access,
            "expires_at": expires_at,
        }
        self.manager.gitlab.http_post(path, post_data=data, **kwargs)

    @cli.register_custom_action("Project", ("group_id",))
    @exc.on_http_error(exc.GitlabDeleteError)
    def unshare(self, group_id: int, **kwargs: Any) -> None:
        """Delete a shared project link within a group.

        Args:
            group_id: ID of the group.
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabDeleteError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/share/{group_id}"
        self.manager.gitlab.http_delete(path, **kwargs)

    # variables not supported in CLI
    @cli.register_custom_action("Project", ("ref", "token"))
    @exc.on_http_error(exc.GitlabCreateError)
    def trigger_pipeline(
        self,
        ref: str,
        token: str,
        variables: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> ProjectPipeline:
        """Trigger a CI build.

        See https://gitlab.com/help/ci/triggers/README.md#trigger-a-build

        Args:
            ref: Commit to build; can be a branch name or a tag
            token: The trigger token
            variables: Variables passed to the build script
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server failed to perform the request
        """
        variables = variables or {}
        path = f"/projects/{self.encoded_id}/trigger/pipeline"
        post_data = {"ref": ref, "token": token, "variables": variables}
        attrs = self.manager.gitlab.http_post(path, post_data=post_data, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(attrs, dict)
        return ProjectPipeline(self.pipelines, attrs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabHousekeepingError)
    def housekeeping(self, **kwargs: Any) -> None:
        """Start the housekeeping task.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabHousekeepingError: If the server failed to perform the
                                     request
        """
        path = f"/projects/{self.encoded_id}/housekeeping"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabRestoreError)
    def restore(self, **kwargs: Any) -> None:
        """Restore a project marked for deletion.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabRestoreError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/restore"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("Project", optional=("wiki",))
    @exc.on_http_error(exc.GitlabGetError)
    def snapshot(
        self,
        wiki: bool = False,
        streamed: bool = False,
        action: Optional[Callable[[bytes], None]] = None,
        chunk_size: int = 1024,
        *,
        iterator: bool = False,
        **kwargs: Any,
    ) -> Optional[Union[bytes, Iterator[Any]]]:
        """Return a snapshot of the repository.

        Args:
            wiki: If True return the wiki repository
            streamed: If True the data will be processed by chunks of
                `chunk_size` and each chunk is passed to `action` for
                treatment.
            iterator: If True directly return the underlying response
                iterator
            action: Callable responsible of dealing with chunk of
                data
            chunk_size: Size of each chunk
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the content could not be retrieved

        Returns:
            The uncompressed tar archive of the repository
        """
        path = f"/projects/{self.encoded_id}/snapshot"
        result = self.manager.gitlab.http_get(
            path, streamed=streamed, raw=True, wiki=wiki, **kwargs
        )
        if TYPE_CHECKING:
            assert isinstance(result, requests.Response)
        return utils.response_content(
            result, streamed, action, chunk_size, iterator=iterator
        )

    @cli.register_custom_action("Project", ("scope", "search"))
    @exc.on_http_error(exc.GitlabSearchError)
    def search(
        self, scope: str, search: str, **kwargs: Any
    ) -> Union[client.GitlabList, List[Dict[str, Any]]]:
        """Search the project resources matching the provided string.'

        Args:
            scope: Scope of the search
            search: Search string
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabSearchError: If the server failed to perform the request

        Returns:
            A list of dicts describing the resources found.
        """
        data = {"scope": scope, "search": search}
        path = f"/projects/{self.encoded_id}/search"
        return self.manager.gitlab.http_list(path, query_data=data, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabCreateError)
    def mirror_pull(self, **kwargs: Any) -> None:
        """Start the pull mirroring process for the project.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabCreateError: If the server failed to perform the request
        """
        path = f"/projects/{self.encoded_id}/mirror/pull"
        self.manager.gitlab.http_post(path, **kwargs)

    @cli.register_custom_action("Project")
    @exc.on_http_error(exc.GitlabGetError)
    def mirror_pull_details(self, **kwargs: Any) -> Dict[str, Any]:
        """Get a project's pull mirror details.

        Introduced in GitLab 15.5.

        Args:
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabGetError: If the server failed to perform the request

        Returns:
            dict of the parsed json returned by the server
        """
        path = f"/projects/{self.encoded_id}/mirror/pull"
        result = self.manager.gitlab.http_get(path, **kwargs)
        if TYPE_CHECKING:
            assert isinstance(result, dict)
        return result

    @cli.register_custom_action("Project", ("to_namespace",))
    @exc.on_http_error(exc.GitlabTransferProjectError)
    def transfer(self, to_namespace: Union[int, str], **kwargs: Any) -> None:
        """Transfer a project to the given namespace ID

        Args:
            to_namespace: ID or path of the namespace to transfer the
            project to
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabTransferProjectError: If the project could not be transferred
        """
        path = f"/projects/{self.encoded_id}/transfer"
        self.manager.gitlab.http_put(
            path, post_data={"namespace": to_namespace}, **kwargs
        )


class ProjectManager(CRUDMixin, RESTManager):
    _path = "/projects"
    _obj_cls = Project
    # Please keep these _create_attrs in same order as they are at:
    # https://docs.gitlab.com/ee/api/projects.html#create-project
    _create_attrs = RequiredOptional(
        optional=(
            "name",
            "path",
            "allow_merge_on_skipped_pipeline",
            "only_allow_merge_if_all_status_checks_passed",
            "analytics_access_level",
            "approvals_before_merge",
            "auto_cancel_pending_pipelines",
            "auto_devops_deploy_strategy",
            "auto_devops_enabled",
            "autoclose_referenced_issues",
            "avatar",
            "build_coverage_regex",
            "build_git_strategy",
            "build_timeout",
            "builds_access_level",
            "ci_config_path",
            "container_expiration_policy_attributes",
            "container_registry_access_level",
            "container_registry_enabled",
            "default_branch",
            "description",
            "emails_disabled",
            "external_authorization_classification_label",
            "forking_access_level",
            "group_with_project_templates_id",
            "import_url",
            "initialize_with_readme",
            "issues_access_level",
            "issues_enabled",
            "jobs_enabled",
            "lfs_enabled",
            "merge_method",
            "merge_pipelines_enabled",
            "merge_requests_access_level",
            "merge_requests_enabled",
            "mirror_trigger_builds",
            "mirror",
            "namespace_id",
            "operations_access_level",
            "only_allow_merge_if_all_discussions_are_resolved",
            "only_allow_merge_if_pipeline_succeeds",
            "packages_enabled",
            "pages_access_level",
            "requirements_access_level",
            "printing_merge_request_link_enabled",
            "public_builds",
            "releases_access_level",
            "environments_access_level",
            "feature_flags_access_level",
            "infrastructure_access_level",
            "monitor_access_level",
            "remove_source_branch_after_merge",
            "repository_access_level",
            "repository_storage",
            "request_access_enabled",
            "resolve_outdated_diff_discussions",
            "security_and_compliance_access_level",
            "shared_runners_enabled",
            "show_default_award_emojis",
            "snippets_access_level",
            "snippets_enabled",
            "squash_option",
            "tag_list",
            "topics",
            "template_name",
            "template_project_id",
            "use_custom_template",
            "visibility",
            "wiki_access_level",
            "wiki_enabled",
        ),
    )
    # Please keep these _update_attrs in same order as they are at:
    # https://docs.gitlab.com/ee/api/projects.html#edit-project
    _update_attrs = RequiredOptional(
        optional=(
            "allow_merge_on_skipped_pipeline",
            "only_allow_merge_if_all_status_checks_passed",
            "analytics_access_level",
            "approvals_before_merge",
            "auto_cancel_pending_pipelines",
            "auto_devops_deploy_strategy",
            "auto_devops_enabled",
            "autoclose_referenced_issues",
            "avatar",
            "build_coverage_regex",
            "build_git_strategy",
            "build_timeout",
            "builds_access_level",
            "ci_config_path",
            "ci_default_git_depth",
            "ci_forward_deployment_enabled",
            "ci_allow_fork_pipelines_to_run_in_parent_project",
            "ci_separated_caches",
            "container_expiration_policy_attributes",
            "container_registry_access_level",
            "container_registry_enabled",
            "default_branch",
            "description",
            "emails_disabled",
            "enforce_auth_checks_on_uploads",
            "external_authorization_classification_label",
            "forking_access_level",
            "import_url",
            "issues_access_level",
            "issues_enabled",
            "issues_template",
            "jobs_enabled",
            "keep_latest_artifact",
            "lfs_enabled",
            "merge_commit_template",
            "merge_method",
            "merge_pipelines_enabled",
            "merge_requests_access_level",
            "merge_requests_enabled",
            "merge_requests_template",
            "merge_trains_enabled",
            "mirror_overwrites_diverged_branches",
            "mirror_trigger_builds",
            "mirror_user_id",
            "mirror",
            "mr_default_target_self",
            "name",
            "operations_access_level",
            "only_allow_merge_if_all_discussions_are_resolved",
            "only_allow_merge_if_pipeline_succeeds",
            "only_mirror_protected_branches",
            "packages_enabled",
            "pages_access_level",
            "requirements_access_level",
            "restrict_user_defined_variables",
            "path",
            "public_builds",
            "releases_access_level",
            "environments_access_level",
            "feature_flags_access_level",
            "infrastructure_access_level",
            "monitor_access_level",
            "remove_source_branch_after_merge",
            "repository_access_level",
            "repository_storage",
            "request_access_enabled",
            "resolve_outdated_diff_discussions",
            "security_and_compliance_access_level",
            "service_desk_enabled",
            "shared_runners_enabled",
            "show_default_award_emojis",
            "snippets_access_level",
            "snippets_enabled",
            "issue_branch_template",
            "squash_commit_template",
            "squash_option",
            "suggestion_commit_message",
            "tag_list",
            "topics",
            "visibility",
            "wiki_access_level",
            "wiki_enabled",
        ),
    )
    _list_filters = (
        "archived",
        "id_after",
        "id_before",
        "last_activity_after",
        "last_activity_before",
        "membership",
        "min_access_level",
        "order_by",
        "owned",
        "repository_checksum_failed",
        "repository_storage",
        "search_namespaces",
        "search",
        "simple",
        "sort",
        "starred",
        "statistics",
        "topic",
        "visibility",
        "wiki_checksum_failed",
        "with_custom_attributes",
        "with_issues_enabled",
        "with_merge_requests_enabled",
        "with_programming_language",
    )
    _types = {
        "avatar": types.ImageAttribute,
        "topic": types.CommaSeparatedListAttribute,
        "topics": types.ArrayAttribute,
    }

    def get(self, id: Union[str, int], lazy: bool = False, **kwargs: Any) -> Project:
        return cast(Project, super().get(id=id, lazy=lazy, **kwargs))

    @exc.on_http_error(exc.GitlabImportError)
    def import_project(
        self,
        file: str,
        path: str,
        name: Optional[str] = None,
        namespace: Optional[str] = None,
        overwrite: bool = False,
        override_params: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> Union[Dict[str, Any], requests.Response]:
        """Import a project from an archive file.

        Args:
            file: Data or file object containing the project
            path: Name and path for the new project
            name: The name of the project to import. If not provided,
                defaults to the path of the project.
            namespace: The ID or path of the namespace that the project
                will be imported to
            overwrite: If True overwrite an existing project with the
                same path
            override_params: Set the specific settings for the project
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabImportError: If the server failed to perform the request

        Returns:
            A representation of the import status.
        """
        files = {"file": ("file.tar.gz", file, "application/octet-stream")}
        data = {"path": path, "overwrite": str(overwrite)}
        if override_params:
            for k, v in override_params.items():
                data[f"override_params[{k}]"] = v
        if name is not None:
            data["name"] = name
        if namespace:
            data["namespace"] = namespace
        return self.gitlab.http_post(
            "/projects/import", post_data=data, files=files, **kwargs
        )

    @exc.on_http_error(exc.GitlabImportError)
    def remote_import(
        self,
        url: str,
        path: str,
        name: Optional[str] = None,
        namespace: Optional[str] = None,
        overwrite: bool = False,
        override_params: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> Union[Dict[str, Any], requests.Response]:
        """Import a project from an archive file stored on a remote URL.

        Args:
            url: URL for the file containing the project data to import
            path: Name and path for the new project
            name: The name of the project to import. If not provided,
                defaults to the path of the project.
            namespace: The ID or path of the namespace that the project
                will be imported to
            overwrite: If True overwrite an existing project with the
                same path
            override_params: Set the specific settings for the project
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabImportError: If the server failed to perform the request

        Returns:
            A representation of the import status.
        """
        data = {"path": path, "overwrite": str(overwrite), "url": url}
        if override_params:
            for k, v in override_params.items():
                data[f"override_params[{k}]"] = v
        if name is not None:
            data["name"] = name
        if namespace:
            data["namespace"] = namespace
        return self.gitlab.http_post(
            "/projects/remote-import", post_data=data, **kwargs
        )

    @exc.on_http_error(exc.GitlabImportError)
    def remote_import_s3(
        self,
        path: str,
        region: str,
        bucket_name: str,
        file_key: str,
        access_key_id: str,
        secret_access_key: str,
        name: Optional[str] = None,
        namespace: Optional[str] = None,
        overwrite: bool = False,
        override_params: Optional[Dict[str, Any]] = None,
        **kwargs: Any,
    ) -> Union[Dict[str, Any], requests.Response]:
        """Import a project from an archive file stored on AWS S3.

        Args:
            region: AWS S3 region name where the file is stored
            bucket_name: AWS S3 bucket name where the file is stored
            file_key: AWS S3 file key to identify the file.
            access_key_id: AWS S3 access key ID.
            secret_access_key: AWS S3 secret access key.
            path: Name and path for the new project
            name: The name of the project to import. If not provided,
                defaults to the path of the project.
            namespace: The ID or path of the namespace that the project
                will be imported to
            overwrite: If True overwrite an existing project with the
                same path
            override_params: Set the specific settings for the project
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabImportError: If the server failed to perform the request

        Returns:
            A representation of the import status.
        """
        data = {
            "region": region,
            "bucket_name": bucket_name,
            "file_key": file_key,
            "access_key_id": access_key_id,
            "secret_access_key": secret_access_key,
            "path": path,
            "overwrite": str(overwrite),
        }
        if override_params:
            for k, v in override_params.items():
                data[f"override_params[{k}]"] = v
        if name is not None:
            data["name"] = name
        if namespace:
            data["namespace"] = namespace
        return self.gitlab.http_post(
            "/projects/remote-import-s3", post_data=data, **kwargs
        )

    def import_bitbucket_server(
        self,
        bitbucket_server_url: str,
        bitbucket_server_username: str,
        personal_access_token: str,
        bitbucket_server_project: str,
        bitbucket_server_repo: str,
        new_name: Optional[str] = None,
        target_namespace: Optional[str] = None,
        **kwargs: Any,
    ) -> Union[Dict[str, Any], requests.Response]:
        """Import a project from BitBucket Server to Gitlab (schedule the import)

        This method will return when an import operation has been safely queued,
        or an error has occurred. After triggering an import, check the
        ``import_status`` of the newly created project to detect when the import
        operation has completed.

        .. note::
            This request may take longer than most other API requests.
            So this method will specify a 60 second default timeout if none is
            specified.
            A timeout can be specified via kwargs to override this functionality.

        Args:
            bitbucket_server_url: Bitbucket Server URL
            bitbucket_server_username: Bitbucket Server Username
            personal_access_token: Bitbucket Server personal access
                token/password
            bitbucket_server_project: Bitbucket Project Key
            bitbucket_server_repo: Bitbucket Repository Name
            new_name: New repository name (Optional)
            target_namespace: Namespace to import repository into.
                Supports subgroups like /namespace/subgroup (Optional)
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabListError: If the server failed to perform the request

        Returns:
            A representation of the import status.

        Example:

        .. code-block:: python

            gl = gitlab.Gitlab_from_config()
            print("Triggering import")
            result = gl.projects.import_bitbucket_server(
                bitbucket_server_url="https://some.server.url",
                bitbucket_server_username="some_bitbucket_user",
                personal_access_token="my_password_or_access_token",
                bitbucket_server_project="my_project",
                bitbucket_server_repo="my_repo",
                new_name="gl_project_name",
                target_namespace="gl_project_path"
            )
            project = gl.projects.get(ret['id'])
            print("Waiting for import to complete")
            while project.import_status == u'started':
                time.sleep(1.0)
                project = gl.projects.get(project.id)
            print("BitBucket import complete")

        """
        data = {
            "bitbucket_server_url": bitbucket_server_url,
            "bitbucket_server_username": bitbucket_server_username,
            "personal_access_token": personal_access_token,
            "bitbucket_server_project": bitbucket_server_project,
            "bitbucket_server_repo": bitbucket_server_repo,
        }
        if new_name:
            data["new_name"] = new_name
        if target_namespace:
            data["target_namespace"] = target_namespace
        if (
            "timeout" not in kwargs
            or self.gitlab.timeout is None
            or self.gitlab.timeout < 60.0
        ):
            # Ensure that this HTTP request has a longer-than-usual default timeout
            # The base gitlab object tends to have a default that is <10 seconds,
            # and this is too short for this API command, typically.
            # On the order of 24 seconds has been measured on a typical gitlab instance.
            kwargs["timeout"] = 60.0
        result = self.gitlab.http_post(
            "/import/bitbucket_server", post_data=data, **kwargs
        )
        return result

    def import_github(
        self,
        personal_access_token: str,
        repo_id: int,
        target_namespace: str,
        new_name: Optional[str] = None,
        github_hostname: Optional[str] = None,
        optional_stages: Optional[Dict[str, bool]] = None,
        **kwargs: Any,
    ) -> Union[Dict[str, Any], requests.Response]:
        """Import a project from Github to Gitlab (schedule the import)

        This method will return when an import operation has been safely queued,
        or an error has occurred. After triggering an import, check the
        ``import_status`` of the newly created project to detect when the import
        operation has completed.

        .. note::
            This request may take longer than most other API requests.
            So this method will specify a 60 second default timeout if none is
            specified.
            A timeout can be specified via kwargs to override this functionality.

        Args:
            personal_access_token: GitHub personal access token
            repo_id: Github repository ID
            target_namespace: Namespace to import repo into
            new_name: New repo name (Optional)
            github_hostname: Custom GitHub Enterprise hostname.
                Do not set for GitHub.com. (Optional)
            optional_stages: Additional items to import. (Optional)
            **kwargs: Extra options to send to the server (e.g. sudo)

        Raises:
            GitlabAuthenticationError: If authentication is not correct
            GitlabListError: If the server failed to perform the request

        Returns:
            A representation of the import status.

        Example:

        .. code-block:: python

            gl = gitlab.Gitlab_from_config()
            print("Triggering import")
            result = gl.projects.import_github(ACCESS_TOKEN,
                                               123456,
                                               "my-group/my-subgroup")
            project = gl.projects.get(ret['id'])
            print("Waiting for import to complete")
            while project.import_status == u'started':
                time.sleep(1.0)
                project = gl.projects.get(project.id)
            print("Github import complete")

        """
        data = {
            "personal_access_token": personal_access_token,
            "repo_id": repo_id,
            "target_namespace": target_namespace,
            "new_name": new_name,
            "github_hostname": github_hostname,
            "optional_stages": optional_stages,
        }
        data = utils.remove_none_from_dict(data)

        if (
            "timeout" not in kwargs
            or self.gitlab.timeout is None
            or self.gitlab.timeout < 60.0
        ):
            # Ensure that this HTTP request has a longer-than-usual default timeout
            # The base gitlab object tends to have a default that is <10 seconds,
            # and this is too short for this API command, typically.
            # On the order of 24 seconds has been measured on a typical gitlab instance.
            kwargs["timeout"] = 60.0
        result = self.gitlab.http_post("/import/github", post_data=data, **kwargs)
        return result


class ProjectFork(RESTObject):
    pass


class ProjectForkManager(CreateMixin, ListMixin, RESTManager):
    _path = "/projects/{project_id}/forks"
    _obj_cls = ProjectFork
    _from_parent_attrs = {"project_id": "id"}
    _list_filters = (
        "archived",
        "visibility",
        "order_by",
        "sort",
        "search",
        "simple",
        "owned",
        "membership",
        "starred",
        "statistics",
        "with_custom_attributes",
        "with_issues_enabled",
        "with_merge_requests_enabled",
    )
    _create_attrs = RequiredOptional(optional=("namespace",))

    def create(
        self, data: Optional[Dict[str, Any]] = None, **kwargs: Any
    ) -> ProjectFork:
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
        return cast(ProjectFork, CreateMixin.create(self, data, path=path, **kwargs))


class ProjectRemoteMirror(ObjectDeleteMixin, SaveMixin, RESTObject):
    pass


class ProjectRemoteMirrorManager(
    ListMixin, CreateMixin, UpdateMixin, DeleteMixin, RESTManager
):
    _path = "/projects/{project_id}/remote_mirrors"
    _obj_cls = ProjectRemoteMirror
    _from_parent_attrs = {"project_id": "id"}
    _create_attrs = RequiredOptional(
        required=("url",), optional=("enabled", "only_protected_branches")
    )
    _update_attrs = RequiredOptional(optional=("enabled", "only_protected_branches"))


class ProjectStorage(RefreshMixin, RESTObject):
    pass


class ProjectStorageManager(GetWithoutIdMixin, RESTManager):
    _path = "/projects/{project_id}/storage"
    _obj_cls = ProjectStorage
    _from_parent_attrs = {"project_id": "id"}

    def get(self, **kwargs: Any) -> ProjectStorage:
        return cast(ProjectStorage, super().get(**kwargs))


class SharedProject(RESTObject):
    pass


class SharedProjectManager(ListMixin, RESTManager):
    _path = "/groups/{group_id}/projects/shared"
    _obj_cls = SharedProject
    _from_parent_attrs = {"group_id": "id"}
    _list_filters = (
        "archived",
        "visibility",
        "order_by",
        "sort",
        "search",
        "simple",
        "starred",
        "with_issues_enabled",
        "with_merge_requests_enabled",
        "min_access_level",
        "with_custom_attributes",
    )

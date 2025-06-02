from __future__ import annotations

import datetime
import io
import json
import os
import re
import tarfile
import urllib.parse
import xml.etree.ElementTree as ET
from collections import defaultdict
from collections.abc import Iterable
from typing import TYPE_CHECKING, overload

import gitlab
from gitlab.v4.objects import Project, ProjectJob, ProjectPipelineJob

from tasks.kernel_matrix_testing.tool import info
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.pipeline.tools import GitlabJobStatus

if TYPE_CHECKING:
    from typing import Literal

    from tasks.kernel_matrix_testing.types import Component, KMTArchName, StackOutput, VMConfig


class KMTJob:
    """Abstract class representing a Kernel Matrix Testing job, with common properties and methods for all job types"""

    def __init__(self, job: ProjectPipelineJob, gitlab: Project | None = None):
        self.gitlab = gitlab or get_gitlab_repo()
        self.job = job
        self.is_retried = False  # set to True if this job has been later retried
        self.kmt_pipeline: KMTPipeline | None = None
        self.kmt_subpipeline: KMTSubPipeline | None = None

    @property
    def job_api_object(self) -> ProjectJob:
        return self.gitlab.jobs.get(self.id, lazy=True)

    def __str__(self):
        return f"<KMTJob: {self.name}>"

    def refresh(self) -> None:
        self.job = self.gitlab.jobs.get(self.id)

    @property
    def id(self) -> int:
        return int(self.job.id)

    @property
    def pipeline_id(self) -> int:
        return self.job.pipeline["id"]

    @property
    def name(self) -> str:
        return self.job.name

    @property
    def arch(self) -> KMTArchName:
        return "x86_64" if "x64" in self.name else "arm64"

    @property
    def component(self) -> Component:
        return "system-probe" if "sysprobe" in self.name else "security-agent"

    @property
    def status(self) -> GitlabJobStatus:
        return GitlabJobStatus(self.job.status)

    @property
    def failure_reason(self) -> str:
        return self.job.failure_reason

    @overload
    def artifact_file(self, file: str, ignore_not_found: Literal[True]) -> str | None:  # noqa: U100
        ...

    @overload
    def artifact_file(self, file: str, ignore_not_found: Literal[False] = False) -> str:  # noqa: U100
        ...

    def artifact_file(self, file: str, ignore_not_found: bool = False) -> str | None:
        """Download an artifact file from this job, returning its content as a string (decoded UTF-8)

        file: the path to the file inside the artifact
        ignore_not_found: if True, return None if the file is not found, otherwise raise an error
        """
        data = self.artifact_file_binary(file, ignore_not_found=ignore_not_found)  # type: ignore
        return data.decode('utf-8') if data is not None else None

    @overload
    def artifact_file_binary(self, file: str, ignore_not_found: Literal[True]) -> bytes | None:  # noqa: U100
        ...

    @overload
    def artifact_file_binary(self, file: str, ignore_not_found: Literal[False] = False) -> bytes:  # noqa: U100
        ...

    def artifact_file_binary(self, file: str, ignore_not_found: bool = False) -> bytes | None:
        """Download an artifact file from this job, returning its content as a byte array

        file: the path to the file inside the artifact
        ignore_not_found: if True, return None if the file is not found, otherwise raise an error
        """
        try:
            res = self.gitlab.jobs.get(self.id, lazy=True).artifact(file)

            if not isinstance(res, bytes):
                raise RuntimeError(f"Expected binary data, got {type(res)}")

            return res
        except Exception as e:
            if ignore_not_found:
                return None

            raise RuntimeError(f"Could not retrieve artifact {file}") from e

    def retry(self) -> int:
        """Retry the job, returning the job ID of the retry"""
        response = self.job_api_object.retry()
        return response["id"]

    def cancel(self) -> None:
        """Cancel the job"""
        self.job_api_object.cancel()


class KMTSetupEnvJob(KMTJob):
    """Represent a kmt_setup_env_* job, with properties that allow extracting data from
    the job name and output artifacts
    """

    def __init__(self, job: ProjectJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.associated_test_jobs: list[KMTTestRunJob] = []
        self.cleanup_job: KMTCleanupJob | None = None
        self.dependency_upload_jobs: list[KMTDependencyUploadJob] = []

    @property
    def stack_output(self) -> StackOutput:
        return json.loads(self.artifact_file("stack.output"))

    @property
    def vmconfig(self) -> VMConfig:
        return json.loads(self.artifact_file(f"vmconfig-{self.pipeline_id}-{self.arch}.json"))

    @property
    def seen_ips(self) -> set[str]:
        ips: set[str] = set()

        for iface in [0, 1, 2, 3]:
            virbr_status = self.artifact_file(f"libvirt/dnsmasq/virbr{iface}.status", ignore_not_found=True)
            if virbr_status is None or len(virbr_status.strip()) == 0:
                continue

            for entry in json.loads(virbr_status):
                ip = entry.get('ip-address')
                if ip is not None:
                    ips.add(ip)

        return ips

    def get_vm(self, distro: str, vmset: str) -> tuple[str, str] | None:
        """Return the VM ID and IP that matches a given distro and vmset in this environment job

        Returns None if they're not found
        """
        for _, vmmap in self.stack_output.items():
            for microvm in vmmap['microvms']:
                if microvm['tag'] == distro and vmset in microvm['vmset-tags']:
                    return microvm['id'], microvm['ip']
        return None

    def get_vm_boot_log(self, distro: str, vmset: str) -> str | None:
        """Return the boot log for a given distro and vmset in this setup-env job"""
        vmdata = self.get_vm(distro, vmset)
        if vmdata is None:
            return None
        vmid, _ = vmdata

        dd_repo_id = 4670
        vm_log_name = f"ddvm-ci-{self.id}-{dd_repo_id}-kernel-matrix-testing-{self.component}-{self.arch.replace('_', '-')}-{self.pipeline_id}-{vmid}.log"
        vm_log_path = f"libvirt/log/{vm_log_name}"

        return self.artifact_file(vm_log_path)


def get_test_results_from_tarfile(tar: tarfile.TarFile) -> dict[str, bool | None]:
    reports: list[ET.ElementTree] = []
    for member in tar.getmembers():
        filename = os.path.basename(member.name)
        if filename.endswith(".xml"):
            data = tar.extractfile(member)
            if data is not None:
                reports.append(ET.parse(data))

    results: dict[str, bool | None] = {}
    for report in reports:
        for testsuite in report.findall(".//testsuite"):
            pkgname = testsuite.get("name")

            for testcase in report.findall(".//testcase"):
                name = testcase.get("name")
                if name is not None:
                    failed = len(testcase.findall(".//failure")) > 0
                    skipped = len(testcase.findall(".//skipped")) > 0
                    results[f"{pkgname}:{name}"] = None if skipped else not failed

    return results


class KMTTestRunJob(KMTJob):
    """Represent a kmt_test_* job, with properties that allow extracting data from
    the job name and output artifacts
    """

    def __init__(self, job: ProjectJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.setup_job: KMTSetupEnvJob | None = None
        self.cleanup_job: KMTCleanupJob | None = None

    @property
    def vars(self) -> list[str]:
        match = re.search(r"\[([^\]]+)\]", self.name)
        if match is None:
            raise RuntimeError(f"Invalid job name {self.name}")
        return [x.strip() for x in match.group(1).split(",")]

    @property
    def distro(self) -> str:
        return self.vars[0]

    @property
    def vmset(self) -> str:
        return self.vars[1]

    def get_test_results(self) -> dict[str, bool | None]:
        """Return a dictionary with the results of all tests in this job, indexed by "package_name:testname".
        The values are True if test passed, False if failed, None if skipped.
        """
        junit_archive_name = f"junit-{self.arch}-{self.distro}-{self.vmset}.tar.gz"
        junit_archive = self.artifact_file_binary(f"test/new-e2e/tests/{junit_archive_name}", ignore_not_found=True)
        if junit_archive is None:
            return {}

        bytearr = io.BytesIO(junit_archive)
        tar = tarfile.open(fileobj=bytearr)
        return get_test_results_from_tarfile(tar)

    @property
    def has_failed_dependencies(self) -> bool:
        if self.setup_job is None:
            return False

        return self.setup_job.status == GitlabJobStatus.FAILED or any(
            j.status == GitlabJobStatus.FAILED for j in self.setup_job.dependency_upload_jobs
        )


class KMTCleanupJob(KMTJob):
    """Represents a kmt_cleanup_* job, with properties that allow extracting data from
    the job name and output artifacts
    """

    def __init__(self, job: ProjectPipelineJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.setup_job: KMTSetupEnvJob | None = None


class KMTDependencyUploadJob(KMTJob):
    """Represents a job that upload dependencies to KMT hosts"""

    def __init__(self, job: ProjectPipelineJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.setup_job: KMTSetupEnvJob | None = None
        self.cleanup_job: KMTCleanupJob | None = None


class KMTPipeline:
    """Manages all the KMT jobs for a given Gitlab CI pipeline"""

    def __init__(self, pipeline_id: int | str):
        self.pipeline_id = pipeline_id

        self.id_to_job: dict[int, KMTJob] = {}
        self.subpipelines: dict[tuple[Component, KMTArchName], KMTSubPipeline] = {}
        self.gitlab = get_gitlab_repo()

    @property
    def test_jobs(self) -> Iterable[KMTTestRunJob]:
        for subpipeline in self.subpipelines.values():
            yield from subpipeline.test_jobs

    @property
    def setup_jobs(self) -> Iterable[KMTSetupEnvJob]:
        for subpipeline in self.subpipelines.values():
            yield from subpipeline.setup_jobs

    @property
    def cleanup_jobs(self) -> Iterable[KMTCleanupJob]:
        for subpipeline in self.subpipelines.values():
            yield from subpipeline.cleanup_jobs

    @property
    def dependency_upload_jobs(self) -> Iterable[KMTDependencyUploadJob]:
        for subpipeline in self.subpipelines.values():
            yield from subpipeline.dependency_upload_jobs

    def _get_subpipeline(self, component: Component, arch: KMTArchName) -> KMTSubPipeline:
        key = (component, arch)
        if key not in self.subpipelines:
            self.subpipelines[key] = KMTSubPipeline(self.gitlab, self, component, arch)
        return self.subpipelines[key]

    def retrieve_jobs(self) -> None:
        """Gets all KMT jobs for a given pipeline, separated between setup jobs and test run jobs.

        Also links the corresponding setup jobs for each test run job
        """
        gitlab = get_gitlab_repo()
        jobs = gitlab.pipelines.get(self.pipeline_id, lazy=True).jobs.list(per_page=100, all=True, include_retried=True)

        # keep track of jobs by name, to be able to link retried jobs
        name_to_job: dict[str, list[KMTJob]] = defaultdict(list)

        for job in jobs:
            name = job.name

            if name.startswith("kmt_setup_env"):
                kmt_job = KMTSetupEnvJob(job, self.gitlab)
            elif name.startswith("kmt_run_"):
                kmt_job = KMTTestRunJob(job, self.gitlab)
            elif name.startswith("kmt_") and "cleanup" in name and 'manual' not in name:
                kmt_job = KMTCleanupJob(job, self.gitlab)
            elif job.stage == "kernel_matrix_testing_prepare" and "upload" in name:
                kmt_job = KMTDependencyUploadJob(job, self.gitlab)
            else:
                continue  # Not a KMT job

            subpipeline = self._get_subpipeline(kmt_job.component, kmt_job.arch)
            subpipeline._add_job(kmt_job)
            kmt_job.kmt_subpipeline = subpipeline
            kmt_job.kmt_pipeline = self

            name_to_job[name].append(kmt_job)
            self.id_to_job[kmt_job.id] = kmt_job

        for jobs in name_to_job.values():
            if len(jobs) <= 1:
                continue

            jobs_by_time = sorted(jobs, key=lambda x: x.job.created_at)
            for job in jobs_by_time[:-1]:  # All the jobs but the last have been retried
                job.is_retried = True

        for subpipeline in self.subpipelines.values():
            subpipeline._link_related_jobs()

    def get_job(self, job_id: int | str) -> KMTJob | None:
        return self.id_to_job.get(int(job_id))


class KMTSubPipeline:
    """KMTSubpipeline is a collection of jobs that are part of a specific component and architecture"""

    def __init__(self, gitlab: Project, pipeline: KMTPipeline, component: Component, arch: KMTArchName):
        self.gitlab = gitlab
        self.pipeline = pipeline
        self.component = component
        self.arch = arch

        self.setup_jobs: list[KMTSetupEnvJob] = []
        self.test_jobs: list[KMTTestRunJob] = []
        self.cleanup_jobs: list[KMTCleanupJob] = []
        self.dependency_upload_jobs: list[KMTDependencyUploadJob] = []
        self.id_to_job: dict[int, KMTJob] = {}

    @property
    def name(self) -> str:
        return f"{self.component}-{self.arch}"

    @property
    def last_cleanup_job(self) -> KMTCleanupJob | None:
        if len(self.cleanup_jobs) == 0:
            return None
        return self.cleanup_jobs[-1]

    @property
    def last_setup_job(self) -> KMTSetupEnvJob | None:
        if len(self.setup_jobs) == 0:
            return None
        return self.setup_jobs[-1]

    def __hash__(self) -> int:
        return hash((self.component, self.arch))

    def __eq__(self, other: object) -> bool:
        if not isinstance(other, KMTSubPipeline):
            return False
        return self.component == other.component and self.arch == other.arch

    def _add_job(self, job: KMTJob) -> None:
        if isinstance(job, KMTSetupEnvJob):
            self.setup_jobs.append(job)
        elif isinstance(job, KMTTestRunJob):
            self.test_jobs.append(job)
        elif isinstance(job, KMTCleanupJob):
            self.cleanup_jobs.append(job)
        elif isinstance(job, KMTDependencyUploadJob):
            self.dependency_upload_jobs.append(job)

        self.id_to_job[job.id] = job

    def _link_related_jobs(self) -> None:
        """
        Link all the related jobs in this pipeline, ensuring that test jobs are linked to the
        latest setup and cleanup jobs and viceversa
        """
        # Sort all jobs by ID so that they're sorted by "age"
        self.setup_jobs = sorted(self.setup_jobs, key=lambda x: x.id)
        self.test_jobs = sorted(self.test_jobs, key=lambda x: x.id)
        self.cleanup_jobs = sorted(self.cleanup_jobs, key=lambda x: x.id)
        self.dependency_upload_jobs = sorted(self.dependency_upload_jobs, key=lambda x: x.id)

        # Link the test jobs to the setup and cleanup jobs, using always the
        # latest one to ensure we know the last state of the job
        for job in self.test_jobs:
            job.setup_job = self.last_setup_job
            job.cleanup_job = self.last_cleanup_job

            if self.last_setup_job is not None:
                self.last_setup_job.associated_test_jobs.append(job)

        for setup_job in self.setup_jobs:
            setup_job.cleanup_job = self.last_cleanup_job

        for dependency_upload_job in self.dependency_upload_jobs:
            dependency_upload_job.setup_job = self.last_setup_job

            for setup_job in self.setup_jobs:
                setup_job.dependency_upload_jobs.append(dependency_upload_job)

    def get_job(self, job_id: int | str) -> KMTJob | None:
        return self.id_to_job.get(int(job_id))

    def retry_setup_and_dependency_upload(self) -> list[int]:
        """Retry the setup and dependency upload jobs, and return the list of newly scheduled jobs"""
        jobs_to_retry: list[KMTJob] = []

        # Order is important, we want to retry dependency uploads before
        # we retry the cancel job
        jobs_to_retry.extend(self.setup_jobs)
        jobs_to_retry.extend(self.dependency_upload_jobs)

        retry_only_failed = False
        if self.last_cleanup_job is not None:
            if self.last_cleanup_job.status == GitlabJobStatus.CREATED:
                info(
                    f"[+] Cleanup job {self.last_cleanup_job.name} has not run, which means that instances are still running. We will only retry failed dependency jobs"
                )
                retry_only_failed = True
            else:
                jobs_to_retry.append(self.last_cleanup_job)
                if self.last_cleanup_job.status.is_running():
                    info(f"[+] Cleanup job {self.last_cleanup_job.name} is running, will cancel it and retry")
                    self.last_cleanup_job.cancel()

        retried_jobs: list[int] = []
        for job in jobs_to_retry:
            if job.is_retried:
                continue

            if retry_only_failed and job.status != GitlabJobStatus.FAILED:
                info(f"[+] Skipping job {job.name} as it has not failed")
                continue

            try:
                info(f"[+] Retrying job {job.name}")
                retried_id = job.retry()
            except gitlab.exceptions.GitlabJobRetryError as e:
                info(f"[-] Failed to retry job {job.name}: {e}")
                continue
            retried_jobs.append(int(retried_id))

        return retried_jobs


def get_kmt_dashboard_links() -> None | list:
    stage = os.environ.get("CI_JOB_STAGE")
    pipeline = os.environ.get("CI_PIPELINE_ID")
    branch = os.environ.get("CI_COMMIT_REF_NAME")
    pipeline_start = os.environ.get("CI_PIPELINE_CREATED_AT")

    # Check we're running in Gitlab CI
    if pipeline_start is None or branch is None or pipeline is None or stage is None:
        return None

    # Check this is a KMT job
    if "kernel_matrix_testing" not in stage:
        return None

    try:
        pipeline_start_date = datetime.datetime.fromisoformat(pipeline_start)
    except Exception:
        print(f"Error: Could not parse pipeline start date {pipeline_start}")
        return None

    dashboard_end = pipeline_start_date + datetime.timedelta(hours=4)

    query_args = {
        "fromUser": "false",
        "refresh_mode": "paused",
        "tpl_var_ci.pipeline.id[0]": pipeline,
        "tpl_var_git-branch[0]": branch,
        "from_ts": int(pipeline_start_date.timestamp()) * 1000,
        "to_ts": int(dashboard_end.timestamp()) * 1000,
        "live": "false",
    }

    url = f"https://app.datadoghq.com/dashboard/zs9-uia-gsg?{urllib.parse.urlencode(query_args)}"

    return [
        {
            "external_link": {
                "label": "KMT: Pipeline dashboard",
                "url": url,
            }
        }
    ]

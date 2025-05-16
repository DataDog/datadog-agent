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
from typing import TYPE_CHECKING, overload

from gitlab.v4.objects import Project, ProjectJob, ProjectPipelineJob

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
        self.is_retry_job = False  # set to True if this job is a retry of a previous job

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
    """Represent a kmt_cleanup_* job, with properties that allow extracting data from
    the job name and output artifacts
    """

    def __init__(self, job: ProjectJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.setup_job: KMTSetupEnvJob | None = None


class KMTDependencyUploadJob(KMTJob):
    """Represent a job that upload dependencies to KMT hosts"""

    def __init__(self, job: ProjectJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.setup_job: KMTSetupEnvJob | None = None
        self.cleanup_job: KMTCleanupJob | None = None


class KMTPipeline:
    """Represent a Kernel Matrix Testing pipeline, allowing to retrieve the jobs"""

    def __init__(self, pipeline_id: int | str):
        self.pipeline_id = pipeline_id
        self.setup_jobs: list[KMTSetupEnvJob] = []
        self.test_jobs: list[KMTTestRunJob] = []
        self.cleanup_jobs: list[KMTCleanupJob] = []
        self.dependency_upload_jobs: list[KMTDependencyUploadJob] = []
        self.id_to_job: dict[int, KMTJob] = {}

    def retrieve_jobs(self) -> None:
        """Gets all KMT jobs for a given pipeline, separated between setup jobs and test run jobs.

        Also links the corresponding setup jobs for each test run job
        """
        gitlab = get_gitlab_repo()
        jobs = gitlab.pipelines.get(self.pipeline_id, lazy=True).jobs.list(per_page=100, all=True, include_retried=True)

        # map of (arch, component) -> job
        setup_jobs_map: dict[tuple[str, str], KMTSetupEnvJob] = {}
        cleanup_jobs_map: dict[tuple[str, str], KMTCleanupJob] = {}

        # keep track of jobs by name, to be able to link retried jobs
        name_to_job: dict[str, list[KMTJob]] = defaultdict(list)

        for job in jobs:
            name = job.name
            kmt_job: KMTJob | None = None

            if name.startswith("kmt_setup_env"):
                kmt_job = KMTSetupEnvJob(job, gitlab)
                self.setup_jobs.append(kmt_job)

                key = (kmt_job.arch, kmt_job.component)
                if key not in setup_jobs_map:
                    setup_jobs_map[key] = kmt_job
                elif setup_jobs_map[key].id < kmt_job.id:
                    # Keep only the latest setup job for a given arch/component
                    setup_jobs_map[key] = kmt_job
            elif name.startswith("kmt_run_"):
                kmt_job = KMTTestRunJob(job, gitlab)
                self.test_jobs.append(kmt_job)
            elif name.startswith("kmt_") and "cleanup" in name and 'manual' not in name:
                kmt_job = KMTCleanupJob(job, gitlab)
                self.cleanup_jobs.append(kmt_job)

                key = (kmt_job.arch, kmt_job.component)
                if key not in cleanup_jobs_map:
                    cleanup_jobs_map[key] = kmt_job
                elif cleanup_jobs_map[key].id < kmt_job.id:
                    # Keep only the latest cleanup job for a given arch/component
                    cleanup_jobs_map[key] = kmt_job
            elif job.stage == "kernel_matrix_testing_prepare" and "upload" in name:
                kmt_job = KMTDependencyUploadJob(job, gitlab)
                self.dependency_upload_jobs.append(kmt_job)

            if kmt_job is not None:
                name_to_job[name].append(kmt_job)
                self.id_to_job[kmt_job.id] = kmt_job

        for jobs in name_to_job.values():
            if len(jobs) <= 1:
                continue

            jobs_by_time = sorted(jobs, key=lambda x: x.job.created_at)
            for job in jobs_by_time[:-1]:  # All the jobs but the last have been retried
                job.is_retried = True

            for job in jobs_by_time[1:]:  # All the jobs but the first have been retried
                job.is_retry_job = True

        # link test jobs
        for job in self.test_jobs:
            setup_job = setup_jobs_map.get((job.arch, job.component))
            cleanup_job = cleanup_jobs_map.get((job.arch, job.component))
            job.setup_job = setup_job
            job.cleanup_job = cleanup_job
            setup_job.associated_test_jobs.append(job)

        # link setup jobs
        for setup_job in self.setup_jobs:
            cleanup_job = cleanup_jobs_map.get((setup_job.arch, setup_job.component))
            setup_job.cleanup_job = cleanup_job
            cleanup_job.setup_job = setup_job

        # link dependency upload jobs
        for dependency_upload_job in self.dependency_upload_jobs:
            setup_job = setup_jobs_map.get((dependency_upload_job.arch, dependency_upload_job.component))
            dependency_upload_job.setup_job = setup_job
            setup_job.dependency_upload_jobs.append(dependency_upload_job)

    def get_job(self, job_id: int | str) -> KMTJob | None:
        return self.id_to_job.get(int(job_id))


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

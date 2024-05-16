from __future__ import annotations

import io
import json
import os
import re
import tarfile
import xml.etree.ElementTree as ET
from typing import TYPE_CHECKING, overload

from gitlab.v4.objects import Project, ProjectJob

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo

if TYPE_CHECKING:
    from typing import Literal

    from tasks.kernel_matrix_testing.types import Arch, Component, StackOutput, VMConfig


class KMTJob:
    """Abstract class representing a Kernel Matrix Testing job, with common properties and methods for all job types"""

    def __init__(self, job: ProjectJob, gitlab: Project | None = None):
        self.gitlab = gitlab or get_gitlab_repo()
        self.job = job

    def __str__(self):
        return f"<KMTJob: {self.name}>"

    @property
    def id(self) -> int:
        return self.job.id

    @property
    def pipeline_id(self) -> int:
        return self.job.pipeline["id"]

    @property
    def name(self) -> str:
        return self.job.name

    @property
    def arch(self) -> Arch:
        return "x86_64" if "x64" in self.name else "arm64"

    @property
    def component(self) -> Component:
        return "system-probe" if "sysprobe" in self.name else "security-agent"

    @property
    def status(self) -> str:
        return self.job.status

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

            return res.content
        except Exception as e:
            if ignore_not_found:
                return None

            raise RuntimeError(f"Could not retrieve artifact {file}") from e


class KMTSetupEnvJob(KMTJob):
    """Represent a kmt_setup_env_* job, with properties that allow extracting data from
    the job name and output artifacts
    """

    def __init__(self, job: ProjectJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.associated_test_jobs: list[KMTTestRunJob] = []

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


class KMTTestRunJob(KMTJob):
    """Represent a kmt_test_* job, with properties that allow extracting data from
    the job name and output artifacts
    """

    def __init__(self, job: ProjectJob, gitlab: Project | None = None):
        super().__init__(job, gitlab)
        self.setup_job: KMTSetupEnvJob | None = None

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

    def get_junit_reports(self) -> list[ET.ElementTree]:
        """Return the XML data from all JUnit reports in this job. Does not fail if the file is not found."""
        junit_archive_name = f"junit-{self.arch}-{self.distro}-{self.vmset}.tar.gz"
        junit_archive = self.artifact_file_binary(f"test/kitchen/{junit_archive_name}", ignore_not_found=True)
        if junit_archive is None:
            return []

        bytearr = io.BytesIO(junit_archive)
        tar = tarfile.open(fileobj=bytearr)

        reports: list[ET.ElementTree] = []
        for member in tar.getmembers():
            filename = os.path.basename(member.name)
            if filename.endswith(".xml"):
                data = tar.extractfile(member)
                if data is not None:
                    reports.append(ET.parse(data))

        return reports

    def get_test_results(self) -> dict[str, bool | None]:
        """Return a dictionary with the results of all tests in this job, indexed by "package_name:testname".
        The values are True if test passed, False if failed, None if skipped.
        """
        results: dict[str, bool | None] = {}
        for report in self.get_junit_reports():
            for testsuite in report.findall(".//testsuite"):
                pkgname = testsuite.get("name")

                for testcase in report.findall(".//testcase"):
                    name = testcase.get("name")
                    if name is not None:
                        failed = len(testcase.findall(".//failure")) > 0
                        skipped = len(testcase.findall(".//skipped")) > 0
                        results[f"{pkgname}:{name}"] = None if skipped else not failed

        return results


def get_all_jobs_for_pipeline(pipeline_id: int | str) -> tuple[list[KMTSetupEnvJob], list[KMTTestRunJob]]:
    """Gets all KMT jobs for a given pipeline, separated between setup jobs and test run jobs.

    Also links the corresponding setup jobs for each test run job
    """
    setup_jobs: list[KMTSetupEnvJob] = []
    test_jobs: list[KMTTestRunJob] = []

    gitlab = get_gitlab_repo()
    jobs = gitlab.pipelines.get(pipeline_id, lazy=True).jobs.list(per_page=100, all=True)
    for job in jobs:
        name = job.name
        if name.startswith("kmt_setup_env"):
            setup_jobs.append(KMTSetupEnvJob(job, gitlab))
        elif name.startswith("kmt_run_"):
            test_jobs.append(KMTTestRunJob(job, gitlab))

    # link setup jobs
    for job in test_jobs:
        for setup_job in setup_jobs:
            if job.arch == setup_job.arch and job.component == setup_job.component:
                job.setup_job = setup_job
                setup_job.associated_test_jobs.append(job)
                break

    return setup_jobs, test_jobs

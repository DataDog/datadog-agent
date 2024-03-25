import io
import subprocess
from collections import defaultdict
from enum import Enum


class Test:
    PACKAGE_PREFIX = "github.com/DataDog/datadog-agent/"

    def __init__(self, owners, name, package):
        self.name = name
        self.package = self.__removeprefix(package)
        self.owners = self.__get_owners(owners)

    def __removeprefix(self, package):
        return package[len(self.PACKAGE_PREFIX) :]

    def __find_file(self):
        # Find the *_test.go file in the package folder that has the test
        try:
            output = subprocess.run(
                [f"grep -Rl --include=\"*_test.go\" '{self.name}' '{self.package}'"],
                shell=True,
                stdout=subprocess.PIPE,
            )
            return output.stdout.decode('utf-8').splitlines()[0]
        except Exception as e:
            print(f"Exception '{e}' while finding test {self.name} from package {self.package}.")
            print("Setting file to '.none' to notify Agent Developer Experience")
            return '.none'

    def __get_owners(self, OWNERS):
        owners = OWNERS.of(self.__find_file())
        return [name for (kind, name) in owners if kind == "TEAM"]

    @property
    def key(self):
        return (self.name, self.package)


class FailedJobType(Enum):
    JOB_FAILURE = 1
    INFRA_FAILURE = 2


class FailedJobReason(Enum):
    RUNNER = 1
    KITCHEN_AZURE = 4
    FAILED_JOB_SCRIPT = 5
    GITLAB = 6
    KITCHEN = 7
    EC2_SPOT = 8


class FailedJobs:
    def __init__(self):
        self.mandatory_job_failures = []
        self.optional_job_failures = []
        self.mandatory_infra_job_failures = []
        self.optional_infra_job_failures = []

    def add_failed_job(self, job):
        if job["failure_type"] == FailedJobType.INFRA_FAILURE and job["allow_failure"]:
            self.optional_infra_job_failures.append(job)
        elif job["failure_type"] == FailedJobType.INFRA_FAILURE and not job["allow_failure"]:
            self.mandatory_infra_job_failures.append(job)
        elif job["allow_failure"]:
            self.optional_job_failures.append(job)
        else:
            self.mandatory_job_failures.append(job)

    def all_non_infra_failures(self):
        return self.mandatory_job_failures + self.optional_job_failures

    def all_mandatory_failures(self):
        return self.mandatory_job_failures + self.mandatory_infra_job_failures

    def all_failures(self):
        return (
            self.mandatory_job_failures
            + self.optional_job_failures
            + self.mandatory_infra_job_failures
            + self.optional_infra_job_failures
        )


class SlackMessage:
    JOBS_SECTION_HEADER = "Failed jobs:"
    OPTIONAL_JOBS_SECTION_HEADER = "Failed jobs (allowed to fail):"
    INFRA_SECTION_HEADER = "Infrastructure failures:"
    OPTIONAL_INFRA_SECTION_HEADER = "Infrastructure failures (allowed to fail):"
    TEST_SECTION_HEADER = "Failed unit tests:"
    MAX_JOBS_PER_TEST = 2

    def __init__(self, base: str = "", jobs: FailedJobs = None):
        jobs = jobs if jobs else FailedJobs()
        self.base_message = base
        self.failed_jobs = jobs
        self.failed_tests = defaultdict(list)
        self.coda = ""

    def add_test_failure(self, test, job):
        self.failed_tests[test.key].append(job)

    def __render_jobs_section(self, header: str, jobs: list, buffer: io.StringIO):
        if not jobs:
            return

        print(header, file=buffer)

        jobs_per_stage = defaultdict(list)
        for job in jobs:
            jobs_per_stage[job["stage"]].append(job)

        for stage, jobs in jobs_per_stage.items():
            jobs_info = []
            for job in jobs:
                num_retries = len(job["retry_summary"]) - 1
                job_info = f"<{job['url']}|{job['name']}>"
                if num_retries > 0:
                    job_info += f" ({num_retries} retries)"

                jobs_info.append(job_info)

            print(
                f"- {', '.join(jobs_info)} (`{stage}` stage)",
                file=buffer,
            )

    def __render_tests_section(self, buffer):
        print(self.TEST_SECTION_HEADER, file=buffer)
        for (test_name, test_package), jobs in self.failed_tests.items():
            job_list = ", ".join(f"<{job['url']}|{job['name']}>" for job in jobs[: self.MAX_JOBS_PER_TEST])
            if len(jobs) > self.MAX_JOBS_PER_TEST:
                job_list += f" and {len(jobs) - self.MAX_JOBS_PER_TEST} more"
            print(f"- `{test_name}` from package `{test_package}` (in {job_list})", file=buffer)

    def __str__(self):
        buffer = io.StringIO()
        if self.base_message:
            print(self.base_message, file=buffer)
        self.__render_jobs_section(
            self.JOBS_SECTION_HEADER,
            self.failed_jobs.mandatory_job_failures,
            buffer,
        )
        self.__render_jobs_section(
            self.OPTIONAL_JOBS_SECTION_HEADER,
            self.failed_jobs.optional_job_failures,
            buffer,
        )
        self.__render_jobs_section(
            self.INFRA_SECTION_HEADER,
            self.failed_jobs.mandatory_infra_job_failures,
            buffer,
        )
        self.__render_jobs_section(
            self.OPTIONAL_INFRA_SECTION_HEADER,
            self.failed_jobs.optional_infra_job_failures,
            buffer,
        )
        if self.failed_tests:
            self.__render_tests_section(buffer)
        if self.coda:
            print(self.coda, file=buffer)
        return buffer.getvalue()


class TeamMessage(SlackMessage):
    JOBS_SECTION_HEADER = "Failed jobs you own:"
    OPTIONAL_JOBS_SECTION_HEADER = "Failed jobs (allowed to fail) you own:"
    TEST_SECTION_HEADER = "Failed unit tests you own:"

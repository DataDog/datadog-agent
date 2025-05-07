import subprocess
from enum import Enum

from gitlab.v4.objects import ProjectJob


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
    BRIDGE_FAILURE = 3


class FailedJobReason(Enum):
    RUNNER = 1
    FAILED_JOB_SCRIPT = 5
    GITLAB = 6
    EC2_SPOT = 8
    E2E_INFRA_FAILURE = 9
    FAILED_BRIDGE_JOB = 10

    @staticmethod
    def get_infra_failure_mapping():
        return {
            'runner_system_failure': FailedJobReason.RUNNER,
            'stuck_or_timeout_failure': FailedJobReason.GITLAB,
            'unknown_failure': FailedJobReason.GITLAB,
            'api_failure': FailedJobReason.GITLAB,
            'scheduler_failure': FailedJobReason.GITLAB,
            'stale_schedule': FailedJobReason.GITLAB,
            'data_integrity_failure': FailedJobReason.GITLAB,
        }

    @staticmethod
    def from_gitlab_job_failure_reason(failure_reason: str):
        return FailedJobReason.get_infra_failure_mapping().get(failure_reason, FailedJobReason.GITLAB)


class FailedJobs:
    def __init__(self):
        self.mandatory_job_failures = []
        self.optional_job_failures = []
        self.mandatory_infra_job_failures = []
        self.optional_infra_job_failures = []

    def add_failed_job(self, job: ProjectJob):
        if job.failure_type == FailedJobType.INFRA_FAILURE and job.allow_failure:
            self.optional_infra_job_failures.append(job)
        elif job.failure_type == FailedJobType.INFRA_FAILURE and not job.allow_failure:
            self.mandatory_infra_job_failures.append(job)
        elif job.allow_failure:
            self.optional_job_failures.append(job)
        else:
            self.mandatory_job_failures.append(job)

    def all_mandatory_failures(self):
        return self.mandatory_job_failures + self.mandatory_infra_job_failures

    def all_failures(self):
        return (
            self.mandatory_job_failures
            + self.optional_job_failures
            + self.mandatory_infra_job_failures
            + self.optional_infra_job_failures
        )


class PermissionCheck(Enum):
    """
    Enum to have a choice of permissions as argument to the check-permissions task.
    """

    REPO = 'repo'
    TEAM = 'team'

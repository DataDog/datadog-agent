import io
from collections import defaultdict


class Test:
    PACKAGE_PREFIX = "github.com/DataDog/datadog-agent/"

    def __init__(self, owners, name, package):
        self.name = name
        self.package = self.__removeprefix(package)
        self.owners = self.__get_owners(owners, package)

    def __removeprefix(self, package):
        return package[len(self.PACKAGE_PREFIX) :]

    def __get_owners(self, OWNERS, package):
        owners = OWNERS.of(self.__removeprefix(package))
        return [name for (kind, name) in owners if kind == "TEAM"]

    def __str__(self):
        return "`{}` from package `{}`".format(self.name, self.package)


class SlackMessage:
    JOBS_SECTION_HEADER = "Failed jobs:"
    TEST_SECTION_HEADER = "Failed unit tests:"
    MAX_JOBS_PER_TEST = 2

    def __init__(self, header, jobs=None):
        self.base_message = header
        self.failed_jobs = jobs if jobs else []
        self.failed_tests = defaultdict(list)
        self.coda = ""

    def add_test_failure(self, test, job):
        self.failed_tests[test].append(job)

    def __render_jobs_section(self, buffer):
        print(self.JOBS_SECTION_HEADER, file=buffer)
        for job in self.failed_jobs:
            extra_info = "stage {stage}".format(stage=job["stage"])
            num_retries = len(job["retry_summary"]) - 1
            if num_retries > 0:
                extra_info += ", after {retries} retries".format(retries=num_retries)

            print(
                "- <{url}|{name}> ({extra})".format(url=job["url"], name=job["name"], extra=extra_info), file=buffer,
            )

    def __render_tests_section(self, buffer):
        print(self.TEST_SECTION_HEADER, file=buffer)
        for test, jobs in self.failed_tests.items():
            job_list = ", ".join("<{}|{}>".format(job["url"], job["name"]) for job in jobs[: self.MAX_JOBS_PER_TEST])
            if len(jobs) > self.MAX_JOBS_PER_TEST:
                job_list += "and {} more".format(len(jobs) - self.MAX_JOBS_PER_TEST)
            print("- {} (in {})".format(test, job_list), file=buffer)

    def __str__(self):
        buffer = io.StringIO(self.base_message)
        if self.failed_jobs:
            self.__render_jobs_section(buffer)
        if self.failed_tests:
            self.__render_tests_section(buffer)
        if self.coda:
            print(self.coda, file=buffer)
        return buffer.getvalue()


class TeamMessage(SlackMessage):
    JOBS_SECTION_HEADER = "Failed jobs you own:"
    TEST_SECTION_HEADER = "Failed unit tests you own:"

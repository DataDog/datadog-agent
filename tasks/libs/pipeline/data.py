import re
from collections import defaultdict

from gitlab.v4.objects import ProjectJob, ProjectPipeline, ProjectPipelineBridge

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.utils import Color, color_message
from tasks.libs.types.types import FailedJobReason, FailedJobs, FailedJobType

# fakeintake is test infrastructure, not the system under test: failing to reach it at the
# network level (e.g. the VM being briefly unreachable, or fakeintake not listening yet) is
# an infra flake, not a product bug. Shared with tasks.libs.common.junit_upload_core so the
# job-log-level classifier here and the JUnit-XML-level classifier there never drift apart.
#
# Matches Go's *url.Error text for a fakeintake HTTP call that failed at the network level:
#   Get "http://10.255.119.230:80/fakeintake/rc/stats": dial tcp 10.255.119.230:80: i/o timeout
#   Get "http://10.255.98.252:80/fakeintake/payloads?endpoint=/api/v2/series": dial tcp ...: connect: connection refused
#
# Anchored on the route prefix rather than by excluding /api/... paths: the Agent's own
# forwarder errors use /api/... on the same host and must not be swept in, but /api/... also
# appears inside the ?endpoint= query of a legitimate fakeintake URL (second example above),
# so only the path segment is a reliable discriminator. /debug/ covers GetLastAPIKey, the one
# fakeintake client route not under /fakeintake/.
#
# The error-side alternation lists what real jobs have produced (i/o timeout, connection
# refused -- both seen in the same retry loop, so which one surfaces is a timing coin-flip)
# plus the neighbouring dial, transport and DNS failures of the same "cannot reach
# fakeintake" class. It is still an enumeration, so a form not listed here is a false
# negative; only the Go side could decide this on net.Error instead of on text.
FAKEINTAKE_UNREACHABLE_RE = re.compile(
    r'"https?://[^"\s]*/(?:fakeintake|debug)/[^"\s]*": [^"\n]*'
    r'(?:i/o timeout'
    r'|context deadline exceeded'
    r'|TLS handshake timeout'
    r'|connect: connection refused'
    r'|connection reset by peer'
    r'|no route to host'
    r'|no such host)'
)
# Produced by test/fakeintake/client's c.get() panic on a read-side timeout. Complementary to
# the regex above: c.get() is the only path that panics, every other client method returns the
# bare *url.Error the regex matches.
FAKEINTAKE_TIMEOUT_PANIC = 'fakeintake call timed out'


def get_failed_jobs(pipeline: ProjectPipeline, repo_name: str) -> FailedJobs:
    """
    Retrieves the list of failed jobs for a given pipeline id in a given project.
    """
    repo = get_gitlab_repo(repo_name)
    jobs = pipeline.jobs.list(per_page=100, all=True)
    bridges = pipeline.bridges.list(per_page=100, all=True)
    # Add bridge jobs to the list of jobs, as they can fail the pipeline
    jobs.extend(bridges)

    # Get instances of failed jobs grouped by name
    failed_jobs = defaultdict(list)
    for job in jobs:
        if job.status == "failed":
            failed_jobs[job.name].append(job)

    # There, we now have the following map:
    # job name -> list of jobs with that name, including at least one failed job
    processed_failed_jobs = FailedJobs()
    for job_name, jobs in failed_jobs.items():
        # We sort each list per creation date
        jobs.sort(key=lambda x: x.created_at)
        # We truncate the job name to increase readability
        name = truncate_job_name(job_name)
        job = jobs[-1]
        is_standard_job = not isinstance(job, ProjectPipelineBridge)
        # Check the final job in the list: it contains the current status of the job
        # This excludes jobs that were retried and succeeded
        trace = str(repo.jobs.get(job.id, lazy=True).trace(), 'utf-8') if is_standard_job else ""
        failure_type, failure_reason = get_job_failure_context(job, trace)
        final_status = ProjectJob(
            repo.manager,
            attrs={
                "name": name,
                "full_name": job_name,
                "id": job.id,
                "stage": job.stage,
                "status": job.status,
                "tag_list": job.tag_list if is_standard_job else [],
                "allow_failure": job.allow_failure,
                "web_url": job.web_url,
                "retry_summary": [ijob.status for ijob in jobs],
                "failure_type": failure_type,
                "failure_reason": failure_reason,
            },
        )

        # Also exclude jobs allowed to fail
        if final_status.status == "failed" and should_report_job(name, final_status.allow_failure):
            processed_failed_jobs.add_failed_job(final_status)

    return processed_failed_jobs


infra_failure_logs = [
    # Gitlab errors while pulling image on legacy runners
    (re.compile(r'no basic auth credentials \(.*\)'), FailedJobReason.RUNNER),
    (re.compile(r'net/http: TLS handshake timeout \(.*\)'), FailedJobReason.RUNNER),
    # docker / docker-arm runner init failures
    (re.compile(r'Docker runner job start script failed'), FailedJobReason.RUNNER),
    (
        re.compile(
            r'A disposable runner accepted this job, while it shouldn\'t have\. Runners are meant to run just one job and be terminated\.'
        ),
        FailedJobReason.RUNNER,
    ),
    (
        re.compile(r'WARNING: Failed to pull image with policy "always":.*\(.*\)'),
        FailedJobReason.RUNNER,
    ),
    # k8s Gitlab runner init failures
    (
        re.compile(
            r'Job failed \(system failure\): prepare environment: waiting for pod running: timed out waiting for pod to start'
        ),
        FailedJobReason.RUNNER,
    ),
    (
        re.compile(
            r'Job failed \(system failure\): prepare environment: setting up build pod: Internal error occurred: failed calling webhook'
        ),
        FailedJobReason.RUNNER,
    ),
    # Gitlab 5xx errors
    (
        re.compile(r'fatal: unable to access \'.*\': The requested URL returned error: 5..'),
        FailedJobReason.GITLAB,
    ),
    # End to end tests EC2 Spot instances allocation failures
    (
        re.compile(r'Failed to allocate end to end test EC2 Spot instance after [0-9]+ attempts'),
        FailedJobReason.EC2_SPOT,
    ),
    (
        re.compile(r'Connection to [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+ closed by remote host\.'),
        FailedJobReason.EC2_SPOT,
    ),
    # End to end tests internal infrastructure failures
    (
        re.compile(r'E2E INTERNAL ERROR'),
        FailedJobReason.E2E_INFRA_FAILURE,
    ),
    # Network-level failure talking to fakeintake: test infra, not the system under test
    (
        FAKEINTAKE_UNREACHABLE_RE,
        FailedJobReason.E2E_INFRA_FAILURE,
    ),
    (
        re.compile(re.escape(FAKEINTAKE_TIMEOUT_PANIC)),
        FailedJobReason.E2E_INFRA_FAILURE,
    ),
]


def get_infra_failure_info(job_log: str):
    # No Gitlab trace means infra failure from Gitlab
    if not job_log:
        return FailedJobReason.GITLAB

    for regex, type in infra_failure_logs:
        if regex.search(job_log):
            return type


def get_job_failure_context(job: ProjectJob | ProjectPipelineBridge, job_log: str):
    """
    Parses job logs (provided as a string), and returns the type of failure (infra or job) as well
    as the precise reason why the job failed.
    """
    if isinstance(job, ProjectPipelineBridge):
        return FailedJobType.BRIDGE_FAILURE, FailedJobReason.FAILED_BRIDGE_JOB

    infra_failure_reasons = FailedJobReason.get_infra_failure_mapping().keys()

    if job.failure_reason in infra_failure_reasons:
        return FailedJobType.INFRA_FAILURE, FailedJobReason.from_gitlab_job_failure_reason(job.failure_reason)

    type = get_infra_failure_info(job_log)
    if type:
        return FailedJobType.INFRA_FAILURE, type

    return FailedJobType.JOB_FAILURE, FailedJobReason.FAILED_JOB_SCRIPT


# These jobs are allowed to have their full name in the data.
# They are often matrix/parallel jobs where the dimension values are important.
jobs_allowed_to_have_full_names = [re.compile(r'kmt_run_.+_tests_.*')]


def truncate_job_name(job_name, max_char_per_job=48):
    if any(pattern.fullmatch(job_name) for pattern in jobs_allowed_to_have_full_names):
        return job_name

    # Job header should be before the colon, if there is no colon this won't change job_name
    truncated_job_name = job_name.split(":")[0]
    # We also want to avoid it being too long
    truncated_job_name = truncated_job_name[:max_char_per_job]
    return truncated_job_name


# Those jobs have `allow_failure: true` but still need to be included
# in failure reports
jobs_allowed_to_fail_but_need_report = []


def should_report_job(job_name, allow_failure):
    return not allow_failure or any(pattern.fullmatch(job_name) for pattern in jobs_allowed_to_fail_but_need_report)


def get_jobs_skipped_on_pr(pipeline: ProjectPipeline, failed_jobs: FailedJobs) -> tuple[list[str], str]:
    """
    Returns the list of jobs that were not run in the last PR pipeline, and the url to this pipeline, if any.
    """
    missed_jobs = []
    gh = GithubAPI()
    agent = get_gitlab_repo()
    prs = list(gh._repository.get_commit(pipeline.sha).get_pulls())
    if len(prs) == 0:
        return missed_jobs, ""
    if len(prs) > 1:
        print(
            f"[{color_message('WARNING', Color.ORANGE)}]: Multiple PRs found for commit {pipeline.sha}: {[p.number for p in prs]}"
        )
        return missed_jobs, ""
    pr = prs[0]
    branch = pr.head.ref
    pipelines = agent.pipelines.list(ref=branch, get_all=False)
    if len(pipelines) == 0:
        print(f"No pipeline found for {branch}")
        return missed_jobs, ""
    pr_pipeline = pipelines[0]
    jobs = pr_pipeline.jobs.list(per_page=100, all=True)
    bridges = pr_pipeline.bridges.list(per_page=100, all=True)
    jobs.extend(bridges)
    for failed_job in failed_jobs.all_mandatory_failures():
        generated = False
        for job in jobs:
            if job.name == failed_job.full_name:
                generated = True
                if job.status not in ['failed', 'success']:
                    missed_jobs.append(failed_job.name)
                    break
        if not generated:
            missed_jobs.append(failed_job.name)
    return missed_jobs, pr_pipeline.web_url

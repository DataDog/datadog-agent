import os

from invoke import Context

from tasks.agent_ci_api import run
from tasks.e2e_framework import tool


def destroy_remote_stack_local(stack: str, ctx: Context | None = None):
    # Runs in a multiprocessing pool, which only hands over the stack name, so the
    # context is built here rather than passed in.
    ctx = ctx or Context()

    res = tool.run_pulumi(
        ctx,
        f"destroy --remove --yes --stack {stack}",
        project_dir=False,
        warn=True,
        hide=True,
    )
    if res is None:
        return 1, "", "pulumi destroy produced no result", stack
    return res.exited, res.stdout, res.stderr, stack


def destroy_remote_stack_api(stack: str, ctx: Context | None = None, cancel_first: bool = False):
    """Requests stackcleaner to destroy the stack. With cancel_first, the stack's ongoing operation is canceled
    first, which releases the lock left behind by an interrupted Pulumi process."""

    ctx = ctx or Context()
    attrs = f"stack_name={stack},job_name={os.environ['CI_JOB_NAME']},job_id={os.environ['CI_JOB_ID']},pipeline_id={os.environ['CI_PIPELINE_ID']},ref={os.environ['CI_COMMIT_REF_NAME']},ignore_lock=bool:true,ignore_not_found=bool:true"
    if cancel_first:
        attrs += ",cancel_first=bool:true"

    # There is no real command here, the exit code is 1 on error and stderr the error message
    exit_code = 0
    stderr = ""

    try:
        run(
            ctx,
            "stackcleaner/stack",
            env="prod",
            ty="stackcleaner_workflow_request",
            attrs=attrs,
            timeout=10,
            ignore_timeout_error=True,
        )
    except Exception as e:
        exit_code = 1
        stderr = str(e)

    return exit_code, "Stack cleanup request submitted to stackcleaner" if exit_code == 0 else "", stderr, stack


# Written by the e2e framework stack manager in the E2E output directory, one stack name per line
RECORDED_STACKS_FILE = "pulumi-stacks.txt"
# Pulumi stacks of the e2e CI profile live under this organization/project
CI_STACK_PREFIX = "organization/e2eci/"
# Gitlab statuses of jobs that will never use a stack
FINAL_UNUSED_JOB_STATUSES = {"canceled", "canceling", "skipped"}


def read_recorded_stacks(output_dir: str) -> list[str]:
    """Returns the deduplicated stacks recorded by the e2e framework in output_dir."""

    path = os.path.join(output_dir, RECORDED_STACKS_FILE)
    if not os.path.isfile(path):
        return []
    with open(path) as f:
        return list(dict.fromkeys(line.strip() for line in f if line.strip()))


def init_job_consumers(init_job_name: str, jobs: list) -> list:
    """Returns the jobs using the stacks of an init job, named after it without the `-init` suffix."""

    if not init_job_name.endswith("-init"):
        return []
    consumer_name = init_job_name.removesuffix("-init")
    # Parallel and matrix jobs are named `<name> 1/2` or `<name>: [...]`
    return [
        job
        for job in jobs
        if job.name == consumer_name or job.name.startswith((consumer_name + " ", consumer_name + ":"))
    ]


def stacks_cleanup_reason(job_status: str, init_only: bool, consumers: list | None) -> str | None:
    """Returns why the stacks of a job must be cleaned up from its after_script, or None to keep them.

    The stacks are cleaned up at the end of every job, except for init jobs (E2E_INIT_ONLY) whose stacks are kept
    for the jobs consuming them, unless the init job or all of its consumers were canceled.
    """

    if job_status == "canceled":
        return "the job was canceled"
    if not init_only:
        return "the job is over"
    if consumers and all(job.status in FINAL_UNUSED_JOB_STATUSES for job in consumers):
        return "every job consuming the stacks was canceled: " + ", ".join(job.name for job in consumers)
    return None

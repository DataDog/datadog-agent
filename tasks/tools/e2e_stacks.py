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


def destroy_remote_stack_api(stack: str, ctx: Context | None = None):
    ctx = ctx or Context()

    # There is no real command here, the exit code is 1 on error and stderr the error message
    exit_code = 0
    stderr = ""

    try:
        run(
            ctx,
            "stackcleaner/stack",
            env="prod",
            ty="stackcleaner_workflow_request",
            attrs=f"stack_name={stack},job_name={os.environ['CI_JOB_NAME']},job_id={os.environ['CI_JOB_ID']},pipeline_id={os.environ['CI_PIPELINE_ID']},ref={os.environ['CI_COMMIT_REF_NAME']},ignore_lock=bool:true,ignore_not_found=bool:true",
            timeout=10,
            ignore_timeout_error=True,
        )
    except Exception as e:
        exit_code = 1
        stderr = str(e)

    return exit_code, f"Failed to destroy stack {stack} using the API", stderr, stack

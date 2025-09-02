import os
import subprocess

from invoke import Context

from tasks.agent_ci_api import run


def destroy_remote_stack_local(stack: str):
    res = subprocess.run(["pulumi", "destroy", "--remove", "--yes", "--stack", stack], capture_output=True, text=True)
    return res.returncode, res.stdout, res.stderr, stack


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
        )
    except Exception as e:
        exit_code = 1
        stderr = str(e)

    return exit_code, f"Failed to destroy stack {stack} using the API", stderr, stack

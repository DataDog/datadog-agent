import os

from tasks.api import run


def destroy_remote_stack(ctx, stack: str):
    return run(
        ctx,
        "stackcleaner/stack",
        env="prod",
        ty="stackcleaner_workflow_request",
        attrs=f"stack_name={stack},job_name={os.environ['CI_JOB_NAME']},job_id={os.environ['CI_JOB_ID']},pipeline_id={os.environ['CI_PIPELINE_ID']},ref={os.environ['CI_COMMIT_REF_NAME']},ignore_lock=bool:true,ignore_not_found=bool:true",
    )

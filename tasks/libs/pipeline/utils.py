from typing import cast

from invoke.context import Context
from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import Project, ProjectPipeline


def get_pipeline_id(
    ctx: Context, repo: Project, ref: str | None, pipeline: str | None, pull_request_id: str | None, base: bool
) -> int:
    nargs = int(ref is not None) + int(pipeline is not None) + int(pull_request_id is not None)
    assert nargs == 1, "Exactly one of commit, pipeline or pull_request_id must be provided"

    if pipeline is not None:
        return int(pipeline)

    if ref is not None:
        return get_pipeline_id_from_ref(repo, ref)

    assert pull_request_id is not None

    gh = GithubAPI()
    pr = gh.get_pr(int(pull_request_id))
    if base:
        res = ctx.run(f"git merge-base origin/{pr.base.ref} origin/{pr.head.ref}")
        assert res
        base_ref = res.stdout.strip()
        return get_pipeline_id_from_ref(repo, base_ref)

    return get_pipeline_id_from_ref(repo, pr.head.ref)


def get_pipeline_id_from_ref(repo: Project, ref: str) -> int:
    pipeline = get_pipeline_from_ref(repo, ref).get_id()
    return cast(int, pipeline)


def get_pipeline_from_ref(repo: Project, ref: str) -> ProjectPipeline:
    # Get last updated pipeline
    pipelines = repo.pipelines.list(ref=ref, per_page=1, order_by='updated_at', get_all=False)
    pipelines = cast(list[ProjectPipeline], pipelines)
    if len(pipelines) != 0:
        return pipelines[0]

    pipelines = repo.pipelines.list(sha=ref, per_page=1, order_by='updated_at', get_all=False)
    pipelines = cast(list[ProjectPipeline], pipelines)
    if len(pipelines) != 0:
        return pipelines[0]

    print(f"No pipelines found for {ref}")
    raise Exit(code=1)

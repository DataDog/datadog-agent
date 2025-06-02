# generation.py contains function that are used to dynamically generate gitlab pipelines
import copy


def update_child_job_variables(kept_jobs):
    """
    Update the variables of the jobs to reference the parent pipeline.
    It will replace occurences of CI_PIPELINE_ID, CI_COMMIT_SHA and CI_COMMIT_SHORT_SHA to PARENT_PIPELINE_ID, PARENT_COMMIT_SHA and PARENT_COMMIT_SHORT_SHA respectively.
    When triggering a pipeline the PARENT_PIPELINE_ID and PARENT_COMMIT_SHA variables will be set to the CI_PIPELINE_ID and CI_COMMIT_SHA of the parent pipeline.

    """
    updated_jobs = {}
    # Create n jobs with the same configuration
    for job in kept_jobs:
        new_job = copy.deepcopy(kept_jobs[job])
        if 'variables' in new_job:
            # Variables that reference the parent pipeline should be updated
            for key, value in new_job['variables'].items():
                new_value = value
                if not isinstance(value, str):
                    continue
                if "CI_PIPELINE_ID" in value:
                    new_value = new_value.replace("CI_PIPELINE_ID", "PARENT_PIPELINE_ID")
                if "CI_COMMIT_SHA" in value:
                    new_value = new_value.replace("CI_COMMIT_SHA", "PARENT_COMMIT_SHA")
                if "CI_COMMIT_SHORT_SHA" in value:
                    new_value = new_value.replace("CI_COMMIT_SHORT_SHA", "PARENT_COMMIT_SHORT_SHA")
                new_job['variables'][key] = new_value
        updated_jobs[job] = new_job
    return updated_jobs


def update_needs_parent(needs, deps_to_keep, package_deps=None, package_deps_suffix=""):
    """
    Keep only the dependencies that are in the deps_to_keep and in package_deps lists, and update them to target the parent pipeline.
    If package_deps_suffix is provided, it will be added to the dependencies kept from package_deps.
    """

    if package_deps is None:
        package_deps = []
    new_needs = []
    new_needs_package_deps_set = set()

    def _update_needs_parent(needs):
        for need in needs:
            if isinstance(need, str):
                if need in deps_to_keep:
                    new_needs.append({"pipeline": "$PARENT_PIPELINE_ID", "job": need})
                elif need in package_deps:
                    job_to_add = need + package_deps_suffix if not need.endswith(package_deps_suffix) else need
                    if job_to_add in new_needs_package_deps_set:
                        continue
                    new_needs.append(
                        {
                            "pipeline": "$PARENT_PIPELINE_ID",
                            "job": job_to_add,
                        }
                    )
                    new_needs_package_deps_set.add(job_to_add)
                else:
                    continue
            elif isinstance(need, dict):
                if "job" in need and need["job"] in deps_to_keep:
                    new_needs.append({**need, "pipeline": "$PARENT_PIPELINE_ID"})
                elif "job" in need and need["job"] in package_deps:
                    job_to_add = (
                        need["job"] + package_deps_suffix
                        if not need["job"].endswith(package_deps_suffix)
                        else need["job"]
                    )
                    if job_to_add in new_needs_package_deps_set:
                        continue
                    new_needs.append(
                        {
                            **need,
                            "pipeline": "$PARENT_PIPELINE_ID",
                            "job": job_to_add,
                        }
                    )
                    new_needs_package_deps_set.add(job_to_add)
                else:
                    continue
            elif isinstance(need, list):
                _update_needs_parent(need)

    _update_needs_parent(needs)
    return new_needs


def remove_fields(job, fields=('rules', 'extends', 'retry')):
    """
    Remove the fields from the job configuration.
    By default it will remove rules, extends and retry fields.
    """
    for step in fields:
        if step in job:
            del job[step]

import copy
import os

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.gitlab_api import resolve_gitlab_ci_configuration
from tasks.libs.common.utils import gitlab_section
from tasks.libs.pipeline.generation import remove_fields, update_child_job_variables, update_needs_parent


@task
def generate_fips_e2e_pipeline(ctx, generate_config=False):
    """
    Generate a child pipeline containing all the jobs in e2e stage that the ON_NIGHTLY_FIPS variable is set to true.
    The generated pipeline will have the same configuration as the parent pipeline, but with the E2E_FIPS variable set to true.
    All the references to CI_PIPELINE_ID, CI_COMMIT_SHA and CI_COMMIT_SHORT_SHA will be replaced by PARENT_PIPELINE_ID, PARENT_COMMIT_SHA and PARENT_COMMIT_SHORT_SHA respectively.
    """

    skipped_test_fips = {
        "new-e2e-otel": "TestOTelAgent.*",  # No FIPS + OTel image exists yet so these tests will never succeed
        "new-e2e-amp": ".*/TestJMXFIPSMode|TestJMXFetchNixMtls",  # These tests are explicitly testing the agent when FIPS is disabled
    }

    if generate_config:
        # Read gitlab config
        config = resolve_gitlab_ci_configuration(ctx, ".gitlab-ci.yml")
    else:
        # Read gitlab config, which is computed and stored in compute_gitlab_ci_config job
        if not os.path.exists("artifacts/after.gitlab-ci.yml"):
            raise Exit(
                "The configuration is not stored as artifact. Please ensure you ran the compute_gitlab_ci_config job, or set generate_config to True"
            )
        with open("artifacts/after.gitlab-ci.yml") as f:
            config = yaml.safe_load(f)[".gitlab-ci.yml"]

    # Lets keep only variables and jobs with flake finder variable
    kept_job = {}
    for job, job_details in config.items():
        if (
            'variables' in job_details
            and 'ON_NIGHTLY_FIPS' in job_details['variables']
            and job_details['variables']['ON_NIGHTLY_FIPS'] == "true"
            and not job.startswith(".")
        ):
            kept_job[job] = job_details

    # Remove rules, extends and retry from the jobs, update needs to point to parent pipeline
    for job in kept_job.values():
        remove_fields(job)
        if "needs" in job:
            job["needs"] = update_needs_parent(
                job["needs"],
                deps_to_keep=["go_e2e_deps", "tests_windows_sysprobe_x64", "tests_windows_secagent_x64"],
                package_deps=[
                    "agent_deb-x64-a7-fips",
                    "agent_deb-x64-a7",
                    "windows_msi_and_bosh_zip_x64-a7-fips",
                    "windows_msi_and_bosh_zip_x64-a7",
                    "agent_rpm-x64-a7",
                    "agent_suse-x64-a7",
                ],
                package_deps_suffix="-fips",
            )

    new_jobs = {}
    new_jobs['variables'] = copy.deepcopy(config['variables'])
    new_jobs["default"] = copy.deepcopy(config["default"])
    new_jobs['variables']['PARENT_PIPELINE_ID'] = 'undefined'
    new_jobs['variables']['PARENT_COMMIT_SHA'] = 'undefined'
    new_jobs['variables']['PARENT_COMMIT_SHORT_SHA'] = 'undefined'
    new_jobs['stages'] = ["new-e2e-fips"]

    updated_jobs = update_child_job_variables(kept_job)
    for job, job_details in updated_jobs.items():
        new_jobs[f"{job}-fips"] = job_details
        new_jobs[f"{job}-fips"]["stage"] = "new-e2e-fips"
        new_jobs[f"{job}-fips"]["variables"]["E2E_FIPS"] = (
            "true"  # Add E2E_FIPS variable to the job, to force using FIPS
        )
        if job in skipped_test_fips:
            if "EXTRA_PARAMS" in new_jobs[f"{job}-fips"]["variables"]:
                new_jobs[f"{job}-fips"]["variables"]["EXTRA_PARAMS"] += f' --skip "{skipped_test_fips[job]}"'
            else:
                new_jobs[f"{job}-fips"]["variables"]["EXTRA_PARAMS"] = f'--skip "{skipped_test_fips[job]}"'
        if 'E2E_PRE_INITIALIZED' in new_jobs[f"{job}-fips"]['variables']:
            del new_jobs[f"{job}-fips"]['variables']['E2E_PRE_INITIALIZED']

    with open("fips-e2e-gitlab-ci.yml", "w") as f:
        f.write(yaml.safe_dump(new_jobs))

    with gitlab_section("Fips e2e generated pipeline", collapsed=True):
        print(yaml.safe_dump(new_jobs))


@task
def e2e_running_in_fips_mode_on_nightly(ctx):
    config = resolve_gitlab_ci_configuration(ctx, ".gitlab-ci.yml")
    fips_status = {}
    for job, job_details in config.items():
        if "new-e2e" not in job or job.startswith(".") or job_details["stage"] != "e2e":
            continue
        if (
            'variables' in job_details
            and 'ON_NIGHTLY_FIPS' in job_details['variables']
            and job_details['variables']['ON_NIGHTLY_FIPS'] == "true"
        ):
            fips_status[job] = True
        else:
            fips_status[job] = False
    for job, status in fips_status.items():
        print(f"{job} is running in FIPS mode on nightly: {status}")

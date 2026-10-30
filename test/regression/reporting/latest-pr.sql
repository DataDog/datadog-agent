WITH events AS (
    SELECT
        pr_number, experiment, check_kind, check_name, outcome,
        CAST(parent_pipeline AS BIGINT) AS parent_pipeline_id,
        CAST(ci_job AS BIGINT) AS ci_job_id,
        CAST(ended AS BIGINT) AS analysis_end,
        smp_job_id
    FROM dd.ci_tests(
        columns => ARRAY[
            '@smp.pr_number', '@smp.experiment', '@smp.check_kind',
            '@smp.check_name', '@smp.outcome', '@smp.parent_pipeline_id',
            '@smp.ci_job_id', '@smp.analysis_end', '@smp.job_id'
        ],
        event_type => 'test',
        filter => 'service:datadog-agent-smp-experiment-health @smp.report_kind:experiment_health @smp.report_version:1 @smp.context:pr'
    ) AS (
        pr_number VARCHAR, experiment VARCHAR, check_kind VARCHAR,
        check_name VARCHAR, outcome VARCHAR, parent_pipeline VARCHAR,
        ci_job VARCHAR, ended VARCHAR, smp_job_id VARCHAR
    )
),
runs AS (
    SELECT DISTINCT pr_number, experiment, parent_pipeline_id, ci_job_id, analysis_end, smp_job_id
    FROM events
),
ranked_runs AS (
    SELECT *, ROW_NUMBER() OVER (
        PARTITION BY pr_number, experiment
        ORDER BY parent_pipeline_id DESC, ci_job_id DESC, analysis_end DESC, smp_job_id DESC
    ) AS run_rank
    FROM runs
)
SELECT DISTINCT e.*
FROM events e
JOIN ranked_runs r ON
    e.pr_number = r.pr_number AND e.experiment = r.experiment
    AND e.parent_pipeline_id = r.parent_pipeline_id AND e.ci_job_id = r.ci_job_id
    AND e.analysis_end = r.analysis_end AND e.smp_job_id = r.smp_job_id
WHERE r.run_rank = 1

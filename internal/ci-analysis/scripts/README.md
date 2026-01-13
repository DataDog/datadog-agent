# CI Analysis Data Collection Scripts

This directory contains scripts and tools for collecting CI/CD metrics to analyze the Datadog Agent CI pipeline performance and reliability.

## Overview

These scripts support the CI analysis effort outlined in `CI_ANALYSIS_EVIDENCE_BASED.md`. They collect data from three sources:

1. **Datadog CI Visibility** - Pipeline and test metrics
2. **GitLab API** - Job performance and runner data
3. **Developer Survey** - Productivity impact and pain points

## Quick Start

### Prerequisites

```bash
# Install required Python packages
pip install datadog-api-client python-gitlab pandas requests

# For GitLab project ID, you can find it in the project settings or use:
# https://gitlab.ddbuild.io/DataDog/datadog-agent (look in project page)
```

### 1. Datadog CI Visibility Data

Extract pipeline and test metrics from Datadog CI Visibility.

**Setup:**
```bash
# Get API keys from: https://app.datadoghq.com/organization-settings/api-keys
export DD_API_KEY="your_api_key_here"
export DD_APP_KEY="your_app_key_here"
export DD_SITE="datadoghq.com"  # Or your Datadog site
```

**Run:**
```bash
python datadog_ci_visibility_queries.py \
  --days 180 \
  --output-dir ./ci_data
```

**Output files:**
- `ci_data/pipeline_durations.csv` - P50/P95/P99 durations by branch
- `ci_data/critical_path_jobs.csv` - Performance of blocking jobs
- `ci_data/flaky_tests.csv` - Tests with 5-95% failure rates
- `ci_data/cost_attribution.csv` - Compute time by stage/job

**Note:** The script currently contains placeholders for actual Datadog API calls. You may need to adapt it based on your Datadog CI Visibility setup and available API endpoints.

### 2. GitLab API Data

Extract pipeline, job, and runner metrics from GitLab.

**Setup:**
```bash
# Get token from: https://gitlab.ddbuild.io/-/profile/personal_access_tokens
# Required scopes: api, read_api
export GITLAB_TOKEN="your_gitlab_token_here"
export GITLAB_URL="https://gitlab.ddbuild.io"
```

**Run:**
```bash
python gitlab_api_extraction.py \
  --project-id 14 \
  --days 180 \
  --max-pipelines 1000 \
  --output-dir ./ci_data
```

**Output files:**
- `ci_data/gitlab_pipelines.csv` - Pipeline duration, status, timestamps
- `ci_data/gitlab_jobs.csv` - Job-level metrics
- `ci_data/gitlab_critical_path.csv` - Statistics for critical path jobs
- `ci_data/gitlab_runners.csv` - Runner information

**To find your project ID:**
```bash
# Install gh CLI if not already installed
brew install gh

# Or look in GitLab project settings page
```

### 3. Developer Survey

Distribute the developer experience survey to team members.

**Survey file:** `developer_survey.md`

**Distribution options:**

1. **Google Forms** (Recommended)
   - Copy questions from `developer_survey.md` into Google Forms
   - Send to all datadog-agent contributors (last 6 months)
   - Enable anonymous responses
   - Set deadline: 1 week

2. **Markdown form in GitHub Discussion**
   - Create a GitHub Discussion with the survey
   - Pin it to the repository
   - Encourage responses via Slack

3. **Slack form**
   - Use Slack's form feature
   - Post in #datadog-agent-dev or relevant channels

**Target audience:**
- All contributors with commits in last 6 months
- Estimated: 50-100 developers

**Timeline:**
- Distribution: Week 2 of data collection
- Collection period: 1 week
- Analysis: Week 3

## Data Collection Timeline

### Week 1: Automated Data Collection

**Day 1-2:**
- [ ] Set up API credentials (Datadog + GitLab)
- [ ] Run Datadog CI Visibility queries
- [ ] Run GitLab API extraction
- [ ] Verify data completeness

**Day 3-4:**
- [ ] Data cleaning and validation
- [ ] Generate summary statistics
- [ ] Identify data gaps

**Day 5:**
- [ ] Create initial visualizations
- [ ] Prepare survey distribution list

### Week 2: Developer Survey

**Day 1:**
- [ ] Create Google Form from `developer_survey.md`
- [ ] Test survey with 2-3 developers
- [ ] Finalize questions

**Day 2:**
- [ ] Send survey to all contributors
- [ ] Post in Slack channels
- [ ] Pin GitHub Discussion

**Day 3-7:**
- [ ] Monitor response rate
- [ ] Send reminders mid-week
- [ ] Close survey end of week

### Week 3: Analysis & Reporting

**Day 1-2:**
- [ ] Analyze survey responses
- [ ] Correlate with CI metrics
- [ ] Identify key pain points

**Day 3-4:**
- [ ] Update `CI_ANALYSIS_EVIDENCE_BASED.md` with real data
- [ ] Create executive summary
- [ ] Generate dashboards

**Day 5:**
- [ ] Review with team
- [ ] Present to Director of Engineering
- [ ] Finalize recommendations

## Expected Data Volume

**Datadog CI Visibility:**
- ~180 days × ~100 pipelines/day = ~18,000 pipelines
- ~18,000 pipelines × ~100 jobs/pipeline = ~1.8M jobs
- ~1.8M jobs × ~10 tests/job = ~18M test results
- **Storage:** ~1-2 GB CSV files

**GitLab API:**
- ~1,000 recent pipelines (configurable)
- ~100 jobs per pipeline = ~100k jobs
- ~50-100 runners
- **Storage:** ~100-200 MB CSV files

**Developer Survey:**
- Target: 50-100 responses
- 19 questions per response
- **Storage:** ~1 MB

## Analyzing the Data

Once data is collected, recommended analysis:

### Pipeline Performance
```python
import pandas as pd

# Load data
pipelines = pd.read_csv('ci_data/gitlab_pipelines.csv')

# Calculate P50, P95, P99
print(pipelines['duration'].describe(percentiles=[.5, .95, .99]))

# Failure rate by branch
failure_rate = pipelines.groupby('ref')['status'].apply(
    lambda x: (x == 'failed').sum() / len(x)
)
```

### Critical Path Analysis
```python
# Load jobs
jobs = pd.read_csv('ci_data/gitlab_jobs.csv')

# Filter critical path
critical = jobs[jobs['stage'].isin(['lint', 'source_test', 'binary_build'])]

# Top 10 slowest jobs
slowest = critical.groupby('job_name')['duration'].mean().sort_values(ascending=False).head(10)
print(slowest)
```

### Flaky Test Detection
```python
# Load from Datadog CI Visibility or calculate from GitLab jobs
# Example: Calculate failure rate per test
test_stats = jobs.groupby('job_name').agg({
    'status': lambda x: {
        'total': len(x),
        'failures': (x == 'failed').sum(),
        'failure_rate': (x == 'failed').sum() / len(x)
    }
})

# Filter for flaky pattern (5-95% failure rate)
flaky = test_stats[
    (test_stats['status'].apply(lambda x: x['failure_rate']) > 0.05) &
    (test_stats['status'].apply(lambda x: x['failure_rate']) < 0.95)
]
```

## Troubleshooting

### Datadog API Issues

**Error: "Unauthorized"**
- Verify `DD_API_KEY` and `DD_APP_KEY` are set correctly
- Check keys have CI Visibility read permissions
- Verify `DD_SITE` matches your Datadog instance

**Error: "No data returned"**
- CI Visibility may not be fully instrumented
- Check service name in queries matches your setup
- Try shorter time range (`--days 30`)

### GitLab API Issues

**Error: "401 Unauthorized"**
- Verify `GITLAB_TOKEN` is set correctly
- Check token has `api` and `read_api` scopes
- Token may have expired (create new one)

**Error: "403 Forbidden"**
- You may not have access to the project
- Verify project ID is correct

**Error: "Rate limited"**
- Script has 0.1s delay between requests
- For large data sets, increase delay in code
- Run during off-peak hours

### Survey Issues

**Low response rate:**
- Send personalized reminders
- Emphasize anonymity
- Share how results will be used
- Offer incentive (team lunch, etc.)

**Biased responses:**
- Ensure survey is truly anonymous
- Reach out to quiet contributors directly
- Compare early vs late responses for bias

## Next Steps

After data collection:

1. **Update evidence-based report** with real metrics
2. **Create CI health dashboard** in Datadog
3. **Prioritize improvements** based on data + survey
4. **Implement quick wins** (see `CI_ANALYSIS_EVIDENCE_BASED.md` Part 8)
5. **Measure impact** of improvements over time

## Files in This Directory

- `README.md` - This file
- `datadog_ci_visibility_queries.py` - Datadog CI Visibility data extraction
- `gitlab_api_extraction.py` - GitLab API data extraction
- `developer_survey.md` - Developer experience survey questionnaire

## Support

For questions or issues:
- Check `CI_ANALYSIS_EVIDENCE_BASED.md` for context
- Review script help: `python <script> --help`
- Contact CI analysis team lead

## References

- [Datadog CI Visibility Documentation](https://docs.datadoghq.com/continuous_integration/)
- [GitLab API Documentation](https://docs.gitlab.com/ee/api/)
- [Evidence-Based CI Analysis Report](../../CI_ANALYSIS_EVIDENCE_BASED.md)

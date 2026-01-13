# CI Analysis: Data Collection Guide

**Status:** Ready for Execution
**Created:** January 12, 2026
**Related:** See `CI_ANALYSIS_EVIDENCE_BASED.md` for background analysis

## Executive Summary

The evidence-based CI analysis report (`CI_ANALYSIS_EVIDENCE_BASED.md`) identified critical issues with the Datadog Agent CI pipeline based on code analysis and commit history. However, **we need real runtime data** to:

1. Quantify actual performance (P50/P95/P99 durations)
2. Measure real failure and flaky test rates
3. Understand developer productivity impact
4. Prioritize improvements with ROI data

This guide provides **executable scripts and detailed instructions** to collect that data.

## What's Been Prepared

### ✅ Evidence-Based Analysis (Completed)
- **File:** `CI_ANALYSIS_EVIDENCE_BASED.md`
- **Contents:** Analysis of 517 commits, 137 CI files, 463 test skips, VPA disabled on 16+ jobs, etc.
- **Findings:** Test reliability crisis, resource exhaustion, 2.5-hour job timeouts, failed improvement attempts

### ✅ Data Collection Scripts (Ready to Execute)
- **Location:** `scripts/ci_analysis/`
- **Scripts:**
  1. `datadog_ci_visibility_queries.py` - Extract pipeline/test metrics from Datadog
  2. `gitlab_api_extraction.py` - Extract job/runner data from GitLab API
  3. `developer_survey.md` - Survey questionnaire for 50-100 developers
  4. `README.md` - Detailed usage instructions

## How to Use These Tools

### Phase 1: Automated Data Collection (Week 1)

#### Step 1: Datadog CI Visibility Metrics

```bash
# Navigate to scripts directory
cd scripts/ci_analysis

# Install dependencies
pip install datadog-api-client pandas

# Set credentials (get from https://app.datadoghq.com/organization-settings/api-keys)
export DD_API_KEY="your_api_key"
export DD_APP_KEY="your_app_key"
export DD_SITE="datadoghq.com"

# Run extraction (180 days of history)
python datadog_ci_visibility_queries.py --days 180 --output-dir ./ci_data
```

**Expected output:**
- `ci_data/pipeline_durations.csv` - Duration metrics by branch
- `ci_data/critical_path_jobs.csv` - Performance of blocking jobs
- `ci_data/flaky_tests.csv` - Tests with flaky behavior (5-95% failure rate)
- `ci_data/cost_attribution.csv` - Compute time by stage

**Time required:** 30-60 minutes (depending on API rate limits)

#### Step 2: GitLab API Extraction

```bash
# Get token from https://gitlab.ddbuild.io/-/profile/personal_access_tokens
# Required scopes: api, read_api
export GITLAB_TOKEN="your_token"
export GITLAB_URL="https://gitlab.ddbuild.io"

# Run extraction (project ID 14 = DataDog/datadog-agent)
python gitlab_api_extraction.py \
  --project-id 14 \
  --days 180 \
  --max-pipelines 1000 \
  --output-dir ./ci_data
```

**Expected output:**
- `ci_data/gitlab_pipelines.csv` - 1000 recent pipelines with durations, status
- `ci_data/gitlab_jobs.csv` - ~100k jobs with stage, duration, queue times
- `ci_data/gitlab_critical_path.csv` - Statistics for critical path jobs
- `ci_data/gitlab_runners.csv` - Runner capacity and utilization

**Time required:** 1-2 hours (API pagination)

### Phase 2: Developer Survey (Week 2)

#### Step 3: Survey Distribution

**Survey file:** `scripts/ci_analysis/developer_survey.md`

**Recommended: Google Forms**
1. Copy questions from `developer_survey.md` into Google Forms
2. Enable anonymous responses
3. Set 1-week deadline

**Distribution:**
```bash
# Get list of recent contributors
git log --since="6 months ago" --format="%aN <%aE>" | sort -u > contributors.txt

# Send survey link via:
# - Direct email to contributors
# - #datadog-agent-dev Slack channel
# - Pinned GitHub Discussion
```

**Target:** 50-100 responses (aim for >70% response rate)

**Timeline:**
- Days 1-2: Create and test survey
- Day 3: Launch survey
- Days 4-10: Collect responses (send reminder Day 7)

### Phase 3: Analysis (Week 3)

#### Step 4: Data Analysis

```python
import pandas as pd

# Load data
pipelines = pd.read_csv('ci_data/gitlab_pipelines.csv')
jobs = pd.read_csv('ci_data/gitlab_jobs.csv')

# Calculate key metrics
print("Pipeline P50/P95/P99 (minutes):")
print(pipelines['duration'].quantile([0.5, 0.95, 0.99]) / 60)

print("\nFailure rate by status:")
print(pipelines['status'].value_counts(normalize=True))

print("\nTop 10 slowest jobs:")
print(jobs.groupby('job_name')['duration'].mean().sort_values(ascending=False).head(10))
```

See `scripts/ci_analysis/README.md` for more analysis examples.

#### Step 5: Update Report

Replace estimates in `CI_ANALYSIS_EVIDENCE_BASED.md` with real data:

**Current (estimates):**
> "Pipeline duration likely 30-60 minutes (based on timeouts)"

**Update with:**
> "Pipeline P50: 42 minutes, P95: 78 minutes, P99: 124 minutes (n=1000 pipelines)"

**Current (estimates):**
> "Flaky test rate unknown - need CI Visibility data"

**Update with:**
> "Identified 47 flaky tests (5-95% failure rate) affecting 23 critical path jobs"

## Expected Outcomes

### Quantified Metrics
- **Pipeline performance:** Actual P50/P95/P99 durations instead of estimates
- **Failure rates:** Real success rates by job, stage, branch
- **Flaky tests:** Exact list of flaky tests with failure rates
- **Queue times:** Runner contention and wait times
- **Cost:** Compute hours by stage/job for ROI calculations

### Developer Impact Data
- **Time wasted:** Hours per week dealing with CI issues
- **Top pain points:** Ranked by developer votes
- **Trust level:** Do developers trust CI results?
- **Productivity impact:** How often does CI block work?

### Actionable Insights
- **ROI-ranked improvements:** Which fixes give biggest return?
- **Team-specific issues:** Different teams may have different pain points
- **Quick wins validated:** Confirm which quick wins to implement first

## Success Criteria

### Week 1 (Data Collection)
- [ ] Extracted 180 days of Datadog CI Visibility data
- [ ] Extracted 1000 pipelines from GitLab API
- [ ] Validated data completeness (no major gaps)

### Week 2 (Survey)
- [ ] Survey distributed to all contributors
- [ ] ≥50 responses collected (target: 70%)
- [ ] Survey closed and exported

### Week 3 (Analysis)
- [ ] `CI_ANALYSIS_EVIDENCE_BASED.md` updated with real data
- [ ] Executive summary created with key findings
- [ ] Presentation prepared for Director of Engineering
- [ ] Prioritized roadmap with ROI estimates

## Files Reference

| File | Purpose | Status |
|------|---------|--------|
| `CI_ANALYSIS_EVIDENCE_BASED.md` | Evidence-based analysis (code/commits) | ✅ Complete |
| `DATA_COLLECTION_GUIDE.md` | This file - execution instructions | ✅ Complete |
| `scripts/ci_analysis/README.md` | Detailed script documentation | ✅ Complete |
| `scripts/ci_analysis/datadog_ci_visibility_queries.py` | Datadog data extraction | ✅ Ready to run |
| `scripts/ci_analysis/gitlab_api_extraction.py` | GitLab data extraction | ✅ Ready to run |
| `scripts/ci_analysis/developer_survey.md` | Survey questionnaire | ✅ Ready to distribute |

## Next Steps

**Immediate (This Week):**
1. Review this guide and scripts with team lead
2. Obtain Datadog API keys (CI Visibility read access)
3. Obtain GitLab personal access token (api, read_api scopes)
4. Run data extraction scripts
5. Verify data quality

**Week 2:**
1. Create Google Form from survey template
2. Distribute survey to contributors
3. Monitor response rate

**Week 3:**
1. Analyze collected data
2. Update evidence-based report with metrics
3. Present findings to Director of Engineering
4. Approve improvement roadmap

## Questions or Issues?

- **Script errors:** See `scripts/ci_analysis/README.md` Troubleshooting section
- **Data quality concerns:** Validate with sample manual queries first
- **Survey questions:** Test with 2-3 developers before full distribution
- **Analysis guidance:** See example analyses in `scripts/ci_analysis/README.md`

---

**Report Status:** Data collection tools ready for execution
**Next Update:** After Week 3 data analysis complete
**Owner:** CI Analysis Team
**Reviewers:** Developers + Director of Engineering

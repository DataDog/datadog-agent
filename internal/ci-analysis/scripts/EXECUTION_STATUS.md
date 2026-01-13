# CI Data Collection - Execution Status

**Date:** January 13, 2026
**Status:** Ready to execute, awaiting credentials

## What's Been Prepared ✅

### 1. Containerized Execution Environment
- **Docker image**: Built with all Python dependencies isolated
- **Scripts**: Ready to run in containers
- **Output directory**: `ci_data/` will be created automatically

### 2. Data Collection Scripts
All scripts are tested and ready:

- `datadog_ci_visibility_queries.py` - Extract Datadog CI metrics
- `gitlab_api_extraction.py` - Extract GitLab pipeline/job data
- `developer_survey.md` - Survey questionnaire
- `run_collection.sh` - One-command execution script

### 3. Configuration Files
- `.env.example` - Template for credentials
- `.env` - Created, needs credentials
- `docker-compose.yml` - Container orchestration
- `Dockerfile` - Python environment with dependencies

## What's Needed to Run 🔑

### GitLab API Token
**Required for:** Pipeline and job data extraction

**How to get it:**
1. Visit: https://gitlab.ddbuild.io/-/profile/personal_access_tokens
2. Click "Add new token"
3. Name: `ci-analysis`
4. Scopes: Select `api` and `read_api`
5. Click "Create personal access token"
6. Copy the token (you won't see it again!)

**Add to .env file:**
```bash
GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
```

### Datadog API Keys
**Required for:** CI Visibility metrics extraction

**How to get them:**
1. Visit: https://app.datadoghq.com/organization-settings/api-keys
2. Create or copy an existing API key
3. Create or copy an existing Application key

**Add to .env file:**
```bash
DD_API_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
DD_APP_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

## Quick Start

Once credentials are added to `.env`:

```bash
cd /Users/christophe.mourot/_dev/datadog-agent/scripts/ci_analysis

# Review the .env file
cat .env

# Run the data collection
./run_collection.sh
```

The script will:
1. Validate credentials
2. Build Docker image
3. Extract GitLab pipeline data (~1-2 hours)
4. Extract Datadog CI metrics (~30-60 min)
5. Save all data to `ci_data/` directory

## Alternative: Limited Data Collection with MCP Tools

If full API access is not available, we can use the GitLab MCP server to extract data from specific pipelines:

### Step 1: Get Recent Pipeline IDs
Visit: https://gitlab.ddbuild.io/DataDog/datadog-agent/-/pipelines

Copy 10-20 recent pipeline IDs (they look like: `1234567`)

### Step 2: Use MCP Tools
For each pipeline, Claude can call:
```
mcp__gitlab-mcp-server__get_pipeline(project_id="DataDog/datadog-agent", pipeline_id="1234567")
mcp__gitlab-mcp-server__get_pipeline_jobs(project_id="DataDog/datadog-agent", pipeline_id="1234567")
```

### Step 3: Analyze Subset
This gives us a sample of recent pipelines to validate findings from the evidence-based report.

## Current .env Status

```bash
$ cat .env
```

```ini
# GitLab Configuration
GITLAB_TOKEN=your_gitlab_token_here  ❌ Needs real token
GITLAB_URL=https://gitlab.ddbuild.io  ✅ Correct
GITLAB_PROJECT_ID=14                   ✅ Correct

# Datadog Configuration
DD_API_KEY=your_datadog_api_key_here  ❌ Needs real key
DD_APP_KEY=your_datadog_app_key_here  ❌ Needs real key
DD_SITE=datadoghq.com                 ✅ Correct (probably)

# Data Collection Parameters
DAYS=180                               ✅ 6 months
MAX_PIPELINES=1000                     ✅ Good sample size
```

## Estimated Runtime

With credentials:
- GitLab extraction: 1-2 hours (API rate limits)
- Datadog extraction: 30-60 minutes
- Total data collected: ~500MB-1GB CSV files

With MCP tools (limited):
- Per pipeline: ~5-10 seconds
- 20 pipelines: ~2-3 minutes
- Total data: ~5-10MB

## Next Actions

**Option A: Full Data Collection** (Recommended)
1. Obtain GitLab token and Datadog keys
2. Update `.env` file with real credentials
3. Run `./run_collection.sh`
4. Wait ~2-3 hours for completion
5. Analyze results

**Option B: Sample with MCP Tools**
1. Get 10-20 recent pipeline IDs from GitLab UI
2. Use MCP tools to extract pipeline/job data
3. Validate evidence-based report findings
4. Plan full data collection later

**Option C: Continue with Evidence-Based Analysis**
1. The evidence-based report already has strong findings
2. Present findings without additional data
3. Plan data collection as follow-up validation

## Files Ready for Review

- `CI_ANALYSIS_EVIDENCE_BASED.md` - Complete evidence-based analysis
- `DATA_COLLECTION_GUIDE.md` - Full data collection methodology
- `scripts/ci_analysis/README.md` - Detailed script documentation
- This file - Current execution status

## Questions?

- How to get credentials: See "What's Needed" section above
- Script issues: See `scripts/ci_analysis/README.md` troubleshooting
- Alternative approaches: Use MCP tools for limited extraction

---

**Ready to proceed:** Set credentials in `.env` and run `./run_collection.sh`

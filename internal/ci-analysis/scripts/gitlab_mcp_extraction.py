#!/usr/bin/env python3
"""
GitLab CI Data Extraction using MCP Tools

This script attempts to use the configured GitLab MCP server to extract
pipeline data without requiring manual credential setup.

Note: This is a simpler alternative to gitlab_api_extraction.py that works
with the MCP server if it's configured.
"""

import json
import sys
from pathlib import Path

# This script is designed to work with Claude's MCP GitLab tools
# It generates queries that Claude can execute

def generate_pipeline_queries(project_id: str, num_pipelines: int = 100):
    """Generate queries to extract recent pipeline data"""

    print(f"📊 GitLab CI Data Collection Plan")
    print(f"=" * 60)
    print(f"Project: {project_id}")
    print(f"Target: {num_pipelines} recent pipelines")
    print()

    print("To extract pipeline data, we need to:")
    print()
    print("1. Get recent pipeline IDs from Git history or GitLab UI")
    print(f"   Example: Visit https://gitlab.ddbuild.io/DataDog/datadog-agent/-/pipelines")
    print()
    print("2. For each pipeline ID, use MCP tools:")
    print("   - mcp__gitlab-mcp-server__get_pipeline")
    print("   - mcp__gitlab-mcp-server__get_pipeline_jobs")
    print()
    print("3. For failed jobs, analyze logs:")
    print("   - mcp__gitlab-mcp-server__analyze_job_logs")
    print()

    # Generate example queries
    print("Example MCP tool usage:")
    print("=" * 60)
    print()
    print("# Get pipeline details:")
    print(json.dumps({
        "tool": "mcp__gitlab-mcp-server__get_pipeline",
        "params": {
            "project_id": project_id,
            "pipeline_id": "<pipeline_id>"
        }
    }, indent=2))
    print()
    print("# Get pipeline jobs:")
    print(json.dumps({
        "tool": "mcp__gitlab-mcp-server__get_pipeline_jobs",
        "params": {
            "project_id": project_id,
            "pipeline_id": "<pipeline_id>"
        }
    }, indent=2))
    print()

if __name__ == "__main__":
    project_id = "DataDog/datadog-agent"  # or numeric ID: "14"

    print("⚠️  Note: GitLab MCP tools require specific pipeline IDs")
    print("   We cannot list all pipelines without API credentials or UI access")
    print()

    generate_pipeline_queries(project_id, 100)

#!/usr/bin/env python3
"""
GitLab API Data Extraction Script

This script extracts pipeline, job, and runner metrics from GitLab API
for the datadog-agent repository over the last 6 months.

Prerequisites:
- pip install python-gitlab pandas requests
- Set environment variables:
  - GITLAB_TOKEN: Your GitLab personal access token (with api scope)
  - GITLAB_URL: GitLab instance URL (default: https://gitlab.ddbuild.io)

Usage:
  python gitlab_api_extraction.py --project-id 14 --days 180 --output-dir ./ci_data
"""

import os
import sys
import json
import argparse
from datetime import datetime, timedelta
from typing import List, Dict, Any
import pandas as pd
import time

try:
    import gitlab
    import requests
except ImportError:
    print("ERROR: Required packages not installed")
    print("Install with: pip install python-gitlab pandas requests")
    sys.exit(1)


class GitLabCIAnalyzer:
    """Extracts CI metrics from GitLab API"""

    def __init__(self, gitlab_url: str, token: str):
        self.gl = gitlab.Gitlab(gitlab_url, private_token=token)
        self.gl.auth()
        print(f"✅ Authenticated to GitLab: {gitlab_url}")

    def get_project(self, project_id: int):
        """Get project by ID"""
        try:
            project = self.gl.projects.get(project_id)
            print(f"✅ Found project: {project.name} ({project.path_with_namespace})")
            return project
        except Exception as e:
            print(f"❌ Error getting project {project_id}: {e}")
            sys.exit(1)

    def extract_pipelines(self, project, days: int = 180, max_pipelines: int = 1000) -> pd.DataFrame:
        """
        Extract pipeline statistics
        Returns: pipeline ID, duration, status, created_at, ref, source
        """
        print(f"\n📊 Extracting pipelines (last {days} days, max {max_pipelines})...")

        from datetime import timezone
        cutoff_date = datetime.now(timezone.utc) - timedelta(days=days)
        pipelines_data = []

        try:
            # Fetch pipelines ordered by ID descending (most recent first)
            pipelines = project.pipelines.list(
                order_by='id',
                sort='desc',
                per_page=100,
                all=False  # We'll manually paginate to respect max_pipelines
            )

            count = 0
            page = 1

            while count < max_pipelines:
                print(f"   Fetching page {page} (pipelines: {count}/{max_pipelines})...", end='\r')

                for pipeline in pipelines:
                    # Get full pipeline details
                    full_pipeline = project.pipelines.get(pipeline.id)

                    # Parse created_at
                    created_at = datetime.fromisoformat(full_pipeline.created_at.replace('Z', '+00:00'))

                    # Stop if we've gone past the cutoff date
                    if created_at < cutoff_date:
                        print(f"\n   Reached cutoff date: {cutoff_date}")
                        break

                    pipelines_data.append({
                        'pipeline_id': full_pipeline.id,
                        'status': full_pipeline.status,
                        'ref': full_pipeline.ref,
                        'source': full_pipeline.source,
                        'created_at': full_pipeline.created_at,
                        'updated_at': full_pipeline.updated_at,
                        'duration': full_pipeline.duration,  # seconds
                        'queued_duration': getattr(full_pipeline, 'queued_duration', None),
                        'web_url': full_pipeline.web_url
                    })

                    count += 1
                    if count >= max_pipelines:
                        break

                    # Rate limiting
                    time.sleep(0.1)

                # Check if we should continue
                if count >= max_pipelines or created_at < cutoff_date:
                    break

                # Get next page
                try:
                    pipelines = project.pipelines.list(
                        order_by='id',
                        sort='desc',
                        per_page=100,
                        page=page + 1
                    )
                    page += 1
                except Exception:
                    break

            print(f"\n✅ Extracted {len(pipelines_data)} pipelines")

        except Exception as e:
            print(f"\n❌ Error extracting pipelines: {e}")

        return pd.DataFrame(pipelines_data)

    def extract_jobs(self, project, pipeline_ids: List[int]) -> pd.DataFrame:
        """
        Extract job-level data for given pipelines
        Returns: job ID, name, stage, status, duration, started_at, finished_at, queued_duration
        """
        print(f"\n📊 Extracting jobs for {len(pipeline_ids)} pipelines...")

        jobs_data = []
        total = len(pipeline_ids)

        for idx, pipeline_id in enumerate(pipeline_ids):
            print(f"   Processing pipeline {pipeline_id} ({idx+1}/{total})...", end='\r')

            try:
                pipeline = project.pipelines.get(pipeline_id)
                jobs = pipeline.jobs.list(all=True)

                for job in jobs:
                    jobs_data.append({
                        'pipeline_id': pipeline_id,
                        'job_id': job.id,
                        'job_name': job.name,
                        'stage': job.stage,
                        'status': job.status,
                        'duration': job.duration,  # seconds
                        'queued_duration': getattr(job, 'queued_duration', None),
                        'started_at': job.started_at,
                        'finished_at': job.finished_at,
                        'created_at': job.created_at,
                        'coverage': job.coverage,
                        'runner_id': getattr(job, 'runner', {}).get('id') if hasattr(job, 'runner') and job.runner else None,
                        'web_url': job.web_url
                    })

                # Rate limiting
                time.sleep(0.1)

            except Exception as e:
                print(f"\n⚠️  Error getting jobs for pipeline {pipeline_id}: {e}")
                continue

        print(f"\n✅ Extracted {len(jobs_data)} jobs")
        return pd.DataFrame(jobs_data)

    def extract_runners(self, project) -> pd.DataFrame:
        """
        Extract runner information
        Returns: runner ID, description, status, is_shared, tag_list
        """
        print(f"\n📊 Extracting runner information...")

        runners_data = []

        try:
            # Get project runners
            runners = project.runners.list(all=True)

            for runner in runners:
                runners_data.append({
                    'runner_id': runner.id,
                    'description': runner.description,
                    'status': runner.status,
                    'is_shared': runner.is_shared,
                    'ip_address': runner.ip_address,
                    'tag_list': ','.join(runner.tag_list) if runner.tag_list else '',
                    'run_untagged': runner.run_untagged,
                    'locked': runner.locked,
                    'access_level': runner.access_level,
                    'maximum_timeout': runner.maximum_timeout
                })

            print(f"✅ Extracted {len(runners_data)} runners")

        except Exception as e:
            print(f"❌ Error extracting runners: {e}")

        return pd.DataFrame(runners_data)

    def analyze_critical_path(self, jobs_df: pd.DataFrame) -> pd.DataFrame:
        """
        Analyze critical path stages (lint, source_test, binary_build)
        """
        print(f"\n📊 Analyzing critical path jobs...")

        critical_stages = ['lint', 'source_test', 'binary_build']

        critical_jobs = jobs_df[
            jobs_df['stage'].str.contains('|'.join(critical_stages), case=False, na=False)
        ].copy()

        if critical_jobs.empty:
            print("⚠️  No critical path jobs found")
            return pd.DataFrame()

        # Calculate statistics per job name
        stats = critical_jobs.groupby('job_name').agg({
            'duration': ['count', 'mean', 'median', 'std', 'min', 'max'],
            'status': lambda x: (x == 'success').sum() / len(x) * 100  # success rate
        }).round(2)

        stats.columns = ['_'.join(col).strip() for col in stats.columns.values]
        stats = stats.rename(columns={'status_<lambda>': 'success_rate_pct'})
        stats = stats.reset_index()

        print(f"✅ Analyzed {len(stats)} critical path jobs")

        return stats

    def run_all_extractions(self, project_id: int, days: int, max_pipelines: int, output_dir: str):
        """Execute all extractions and save results"""
        print(f"\n🚀 Starting GitLab CI data extraction")
        print(f"   Project ID: {project_id}")
        print(f"   Time range: Last {days} days")
        print(f"   Max pipelines: {max_pipelines}")
        print(f"   Output directory: {output_dir}")

        os.makedirs(output_dir, exist_ok=True)

        # Get project
        project = self.get_project(project_id)

        # Extract pipelines
        pipelines_df = self.extract_pipelines(project, days, max_pipelines)
        if not pipelines_df.empty:
            output_file = os.path.join(output_dir, "gitlab_pipelines.csv")
            pipelines_df.to_csv(output_file, index=False)
            print(f"✅ Saved: {output_file}")

            # Generate summary statistics
            print("\n📈 Pipeline Statistics:")
            print(f"   Total pipelines: {len(pipelines_df)}")
            print(f"   Status distribution:")
            for status, count in pipelines_df['status'].value_counts().items():
                pct = count / len(pipelines_df) * 100
                print(f"      {status}: {count} ({pct:.1f}%)")

            if 'duration' in pipelines_df.columns:
                durations = pipelines_df['duration'].dropna()
                if not durations.empty:
                    print(f"   Duration (seconds):")
                    print(f"      Mean: {durations.mean():.0f}s ({durations.mean()/60:.1f}m)")
                    print(f"      Median: {durations.median():.0f}s ({durations.median()/60:.1f}m)")
                    print(f"      P95: {durations.quantile(0.95):.0f}s ({durations.quantile(0.95)/60:.1f}m)")
                    print(f"      Max: {durations.max():.0f}s ({durations.max()/60:.1f}m)")

        # Extract jobs
        if not pipelines_df.empty:
            pipeline_ids = pipelines_df['pipeline_id'].tolist()[:100]  # Limit to first 100 for jobs
            jobs_df = self.extract_jobs(project, pipeline_ids)

            if not jobs_df.empty:
                output_file = os.path.join(output_dir, "gitlab_jobs.csv")
                jobs_df.to_csv(output_file, index=False)
                print(f"✅ Saved: {output_file}")

                # Analyze critical path
                critical_stats = self.analyze_critical_path(jobs_df)
                if not critical_stats.empty:
                    output_file = os.path.join(output_dir, "gitlab_critical_path.csv")
                    critical_stats.to_csv(output_file, index=False)
                    print(f"✅ Saved: {output_file}")

        # Extract runners
        runners_df = self.extract_runners(project)
        if not runners_df.empty:
            output_file = os.path.join(output_dir, "gitlab_runners.csv")
            runners_df.to_csv(output_file, index=False)
            print(f"✅ Saved: {output_file}")

        print(f"\n✅ Data extraction complete!")
        print(f"📁 Results saved to: {output_dir}")


def main():
    parser = argparse.ArgumentParser(
        description="Extract CI metrics from GitLab API",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Extract data for project ID 14 (last 180 days)
  python gitlab_api_extraction.py --project-id 14 --days 180 --output-dir ./ci_data

  # Extract recent data only (last 30 days, 500 pipelines)
  python gitlab_api_extraction.py --project-id 14 --days 30 --max-pipelines 500 --output-dir ./ci_data_recent

Environment variables required:
  GITLAB_TOKEN  - Your GitLab personal access token (with api scope)
  GITLAB_URL    - GitLab instance URL (default: https://gitlab.ddbuild.io)

Get your token from: https://gitlab.ddbuild.io/-/profile/personal_access_tokens
Required scopes: api, read_api
        """
    )

    parser.add_argument(
        "--project-id",
        type=int,
        required=True,
        help="GitLab project ID (e.g., 14 for DataDog/datadog-agent)"
    )

    parser.add_argument(
        "--days",
        type=int,
        default=180,
        help="Number of days of history to extract (default: 180)"
    )

    parser.add_argument(
        "--max-pipelines",
        type=int,
        default=1000,
        help="Maximum number of pipelines to extract (default: 1000)"
    )

    parser.add_argument(
        "--output-dir",
        type=str,
        default="./ci_data",
        help="Output directory for CSV files (default: ./ci_data)"
    )

    args = parser.parse_args()

    # Check environment variables
    token = os.getenv("GITLAB_TOKEN")
    gitlab_url = os.getenv("GITLAB_URL", "https://gitlab.ddbuild.io")

    if not token:
        print("❌ ERROR: Missing required environment variable")
        print()
        print("Required:")
        print("  GITLAB_TOKEN - Your GitLab personal access token")
        print()
        print("Optional:")
        print("  GITLAB_URL   - GitLab instance URL (default: https://gitlab.ddbuild.io)")
        print()
        print("Get your token from: https://gitlab.ddbuild.io/-/profile/personal_access_tokens")
        print("Required scopes: api, read_api")
        sys.exit(1)

    # Run extractions
    analyzer = GitLabCIAnalyzer(gitlab_url, token)
    analyzer.run_all_extractions(args.project_id, args.days, args.max_pipelines, args.output_dir)


if __name__ == "__main__":
    main()

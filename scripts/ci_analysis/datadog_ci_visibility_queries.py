#!/usr/bin/env python3
"""
Datadog CI Visibility Data Extraction Script

This script queries Datadog CI Visibility API to extract pipeline and job metrics
for the datadog-agent repository over the last 6 months.

Prerequisites:
- pip install datadog-api-client pandas
- Set environment variables:
  - DD_API_KEY: Your Datadog API key
  - DD_APP_KEY: Your Datadog application key
  - DD_SITE: Your Datadog site (default: datadoghq.com)

Usage:
  python datadog_ci_visibility_queries.py --days 180 --output-dir ./ci_data
"""

import os
import sys
import json
import argparse
from datetime import datetime, timedelta
from typing import List, Dict, Any
import pandas as pd

try:
    from datadog_api_client import ApiClient, Configuration
    from datadog_api_client.v2.api.ci_visibility_pipelines_api import CIVisibilityPipelinesApi
    from datadog_api_client.v2.api.ci_visibility_tests_api import CIVisibilityTestsApi
except ImportError:
    print("ERROR: datadog-api-client not installed")
    print("Install with: pip install datadog-api-client pandas")
    sys.exit(1)


class DatadogCIAnalyzer:
    """Extracts CI metrics from Datadog CI Visibility"""

    def __init__(self, api_key: str, app_key: str, site: str = "datadoghq.com"):
        self.configuration = Configuration()
        self.configuration.api_key["apiKeyAuth"] = api_key
        self.configuration.api_key["appKeyAuth"] = app_key
        self.configuration.server_variables["site"] = site
        self.api_client = ApiClient(self.configuration)

    def query_pipeline_durations(self, days: int = 180) -> pd.DataFrame:
        """
        Query 1: Pipeline duration distribution
        Extract P50, P95, P99 pipeline durations by branch
        """
        print(f"📊 Querying pipeline durations (last {days} days)...")

        with self.api_client as api_client:
            api_instance = CIVisibilityPipelinesApi(api_client)

            # Time range
            end_time = datetime.now()
            start_time = end_time - timedelta(days=days)

            query = f"""
            service:datadog-agent
            @ci.pipeline.name:datadog-agent/datadog-agent
            """

            # In a real implementation, use the aggregation API
            # This is a template showing the structure
            results = []

            try:
                # Placeholder for actual API call
                # response = api_instance.aggregate_ci_app_pipeline_events(
                #     body={
                #         "compute": [
                #             {"aggregation": "p50", "metric": "@duration"},
                #             {"aggregation": "p95", "metric": "@duration"},
                #             {"aggregation": "p99", "metric": "@duration"}
                #         ],
                #         "filter": {
                #             "from": start_time.isoformat(),
                #             "to": end_time.isoformat(),
                #             "query": query
                #         },
                #         "group_by": [
                #             {"facet": "@git.branch", "limit": 100}
                #         ]
                #     }
                # )

                print("⚠️  Actual API implementation needed - see comments in code")
                print(f"   Query: {query.strip()}")
                print(f"   Time range: {start_time} to {end_time}")

            except Exception as e:
                print(f"❌ Error querying pipeline durations: {e}")

        return pd.DataFrame(results)

    def query_critical_path_jobs(self, days: int = 180) -> pd.DataFrame:
        """
        Query 2: Critical path job performance
        Extract duration and failure rates for lint, source_test, binary_build stages
        """
        print(f"📊 Querying critical path jobs (last {days} days)...")

        critical_stages = ["lint", "source_test", "binary_build"]

        query = f"""
        service:datadog-agent
        @ci.stage:({"lint OR source_test OR binary_build"})
        """

        print(f"   Target stages: {', '.join(critical_stages)}")
        print(f"   Query: {query.strip()}")
        print("⚠️  Actual API implementation needed")

        return pd.DataFrame()

    def query_flaky_tests(self, days: int = 180, min_runs: int = 10) -> pd.DataFrame:
        """
        Query 3: Flaky test detection
        Identify tests with failure rate between 5% and 95% (classic flaky pattern)
        """
        print(f"📊 Detecting flaky tests (last {days} days, min {min_runs} runs)...")

        with self.api_client as api_client:
            api_instance = CIVisibilityTestsApi(api_client)

            query = """
            service:datadog-agent
            @test.status:(pass OR fail)
            """

            print(f"   Query: {query.strip()}")
            print(f"   Filtering: 0.05 < failure_rate < 0.95, runs >= {min_runs}")
            print("⚠️  Actual API implementation needed")

            # Algorithm:
            # 1. Group by @test.name
            # 2. Count passes and fails
            # 3. Calculate failure_rate = fails / (passes + fails)
            # 4. Filter where 0.05 < failure_rate < 0.95 and total >= min_runs

        return pd.DataFrame()

    def query_cost_attribution(self, days: int = 180) -> pd.DataFrame:
        """
        Query 4: Cost attribution by stage and job
        Calculate total compute time (duration * resource allocation proxy)
        """
        print(f"📊 Calculating cost attribution (last {days} days)...")

        query = """
        service:datadog-agent
        """

        print(f"   Query: {query.strip()}")
        print(f"   Metrics: duration, aggregated by stage and job")
        print("⚠️  Actual API implementation needed")

        # Note: Resource allocation (CPU/RAM) may not be in CI Visibility
        # May need to join with GitLab config data

        return pd.DataFrame()

    def run_all_queries(self, days: int, output_dir: str):
        """Execute all queries and save results"""
        print(f"\n🚀 Starting CI Visibility data extraction")
        print(f"   Time range: Last {days} days")
        print(f"   Output directory: {output_dir}")
        print()

        os.makedirs(output_dir, exist_ok=True)

        # Query 1: Pipeline durations
        pipeline_df = self.query_pipeline_durations(days)
        if not pipeline_df.empty:
            output_file = os.path.join(output_dir, "pipeline_durations.csv")
            pipeline_df.to_csv(output_file, index=False)
            print(f"✅ Saved: {output_file}")

        print()

        # Query 2: Critical path jobs
        critical_df = self.query_critical_path_jobs(days)
        if not critical_df.empty:
            output_file = os.path.join(output_dir, "critical_path_jobs.csv")
            critical_df.to_csv(output_file, index=False)
            print(f"✅ Saved: {output_file}")

        print()

        # Query 3: Flaky tests
        flaky_df = self.query_flaky_tests(days)
        if not flaky_df.empty:
            output_file = os.path.join(output_dir, "flaky_tests.csv")
            flaky_df.to_csv(output_file, index=False)
            print(f"✅ Saved: {output_file}")

        print()

        # Query 4: Cost attribution
        cost_df = self.query_cost_attribution(days)
        if not cost_df.empty:
            output_file = os.path.join(output_dir, "cost_attribution.csv")
            cost_df.to_csv(output_file, index=False)
            print(f"✅ Saved: {output_file}")

        print()
        print("✅ Data extraction complete!")
        print(f"📁 Results saved to: {output_dir}")


def main():
    parser = argparse.ArgumentParser(
        description="Extract CI metrics from Datadog CI Visibility",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Extract last 180 days of data
  python datadog_ci_visibility_queries.py --days 180 --output-dir ./ci_data

  # Extract last 30 days
  python datadog_ci_visibility_queries.py --days 30 --output-dir ./ci_data_recent

Environment variables required:
  DD_API_KEY    - Your Datadog API key
  DD_APP_KEY    - Your Datadog application key
  DD_SITE       - Your Datadog site (default: datadoghq.com)
        """
    )

    parser.add_argument(
        "--days",
        type=int,
        default=180,
        help="Number of days of history to extract (default: 180)"
    )

    parser.add_argument(
        "--output-dir",
        type=str,
        default="./ci_data",
        help="Output directory for CSV files (default: ./ci_data)"
    )

    args = parser.parse_args()

    # Check environment variables
    api_key = os.getenv("DD_API_KEY")
    app_key = os.getenv("DD_APP_KEY")
    site = os.getenv("DD_SITE", "datadoghq.com")

    if not api_key or not app_key:
        print("❌ ERROR: Missing required environment variables")
        print()
        print("Required:")
        print("  DD_API_KEY - Your Datadog API key")
        print("  DD_APP_KEY - Your Datadog application key")
        print()
        print("Optional:")
        print("  DD_SITE    - Your Datadog site (default: datadoghq.com)")
        print()
        print("Get your API keys from: https://app.datadoghq.com/organization-settings/api-keys")
        sys.exit(1)

    # Run queries
    analyzer = DatadogCIAnalyzer(api_key, app_key, site)
    analyzer.run_all_queries(args.days, args.output_dir)


if __name__ == "__main__":
    main()

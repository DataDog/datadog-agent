#!/usr/bin/env python3
"""Debug script to inspect Datadog CI Visibility API response structure."""

import json
from datetime import datetime, timedelta

from datadog_api_client import ApiClient, Configuration
from datadog_api_client.v2.api.ci_visibility_pipelines_api import CIVisibilityPipelinesApi


def debug_api_response():
    query = 'ci_level:pipeline @ci.pipeline.name:"DataDog/datadog-agent" @git.branch:main'

    with ApiClient(Configuration(enable_retry=True)) as api_client:
        api_instance = CIVisibilityPipelinesApi(api_client)

        response = api_instance.list_ci_app_pipeline_events(
            filter_query=query,
            filter_from=datetime.now() - timedelta(days=7),
            filter_to=datetime.now(),
            page_limit=2,  # Just get 2 for debugging
        )

        if hasattr(response, 'data') and response.data:
            print(f"Found {len(response.data)} events\n")

            for i, event in enumerate(response.data[:1], 1):  # Just look at first one
                print(f"=== Event {i} ===")
                print(f"Event type: {type(event)}")
                print(f"Event attributes type: {type(event.attributes)}")

                # Print all available attributes
                attrs = event.attributes
                print("\nAll attribute keys:")
                if hasattr(attrs, '__dict__'):
                    for key in vars(attrs):
                        print(f"  - {key}")

                # Try to access the data
                print("\nTrying different access patterns:")

                # Pattern 1: Direct attribute access
                print("\n1. Direct attribute access:")
                try:
                    if hasattr(attrs, 'attributes'):
                        inner_attrs = attrs.attributes
                        print(f"  attrs.attributes exists: {type(inner_attrs)}")
                        print(f"  Keys: {list(inner_attrs.keys()) if hasattr(inner_attrs, 'keys') else 'N/A'}")
                except Exception as e:
                    print(f"  Error: {e}")

                # Pattern 2: Check for tags
                print("\n2. Looking for tags:")
                try:
                    if hasattr(attrs, 'tags'):
                        print(f"  attrs.tags: {attrs.tags[:5] if attrs.tags else 'None'}")
                except Exception as e:
                    print(f"  Error: {e}")

                # Pattern 3: Convert to dict
                print("\n3. Convert to dict:")
                try:
                    event_dict = event.to_dict()
                    print(f"  Keys in event dict: {list(event_dict.keys())}")

                    if 'attributes' in event_dict:
                        attrs_dict = event_dict['attributes']
                        print(f"  Keys in attributes: {list(attrs_dict.keys())[:10]}")

                        # Look for pipeline info
                        if 'attributes' in attrs_dict:
                            inner = attrs_dict['attributes']
                            print(f"\n  Inner attributes keys: {list(inner.keys())[:20]}")

                            # Try to find pipeline ID
                            for key in inner.keys():
                                if 'pipeline' in key.lower() or 'id' in key.lower():
                                    print(f"    {key}: {inner[key]}")

                        # Look for common fields
                        print("\n  Looking for common fields:")
                        for field in ['ci.pipeline.id', 'ci.pipeline.url', 'git.branch', 'ci.status', 'start']:
                            if 'attributes' in attrs_dict and field in attrs_dict['attributes']:
                                value = attrs_dict['attributes'][field]
                                print(f"    {field}: {value if len(str(value)) < 100 else str(value)[:100] + '...'}")

                except Exception as e:
                    print(f"  Error: {e}")
                    import traceback

                    traceback.print_exc()

                # Print raw JSON for inspection
                print("\n4. Full event as JSON (first 2000 chars):")
                try:
                    event_json = json.dumps(event.to_dict(), indent=2, default=str)
                    print(event_json[:2000])
                except Exception as e:
                    print(f"  Error: {e}")
        else:
            print("No data in response")


if __name__ == "__main__":
    debug_api_response()

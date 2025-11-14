"""Helper tool for gathering third party license information.

NOTE: This is just a shell of the tool to deal with licenses. The behaviors
      and conditions will be documented later as we learn the requirements.

- Private helper to licenses_csv.bzl
- Cracks open metadata attribute files and deals with them each.
- Produces the rough form of LICENSES.csv
  - TODO: Guess copyrights

TODO:
- We can recognize ship_source_offer, but don't do anythin with it yet.
- figure out how we will merge into the workflow in tasks/license.py
- See if we can have this produce a unified output that could be untarred
  in /opt/datadog/LICENSES so that we don't need pkg_install from each
  library.
"""

import argparse
import json

_DEBUG = 1


class AttrUsage:
    def __init__(self):
        self.licenses = {}

    def load(self, usage_map):
        for file, users in usage_map.items():
            with open(file) as attr_inp:
                self.process_attribute_json(file, json.load(attr_inp))

    def process_attribute_json(self, file, attr):
        """Process a standalone attributes file."""
        if _DEBUG > 0:
            print(f"=== {file}")
        kind = attr.get('kind') or None
        if not kind:
            raise ValueError(f"Badly formated attribute file: {file}\n{content}")

        if kind == "bazel-contrib.supply-chain.attribute.license":
            self.process_license(attr)
            return
        print(f"Unknown attribute type: {kind}")

    def process_package_metadata(self, package_metadata_file):
        """Process a package_metadata bundle file."""
        with open(package_metadata_file) as inp:
            mx = json.load(inp)
            purl = mx.get("purl") or "unset"
            if _DEBUG > 0:
                print(json.dumps(mx, indent="  "))
                print(f"  metadata target: {mx['label']}")
                print(f"  PURL: {purl}")
            if mx.get("attributes"):
                for attr_type, file in mx["attributes"].items():
                    if attr_type == "build.bazel.rules_license.license":
                        with open(file) as attr_inp:
                            self.process_license(json.load(attr_inp), origin=purl)
                    else:
                        print("    ", attr_type, file)

    def process_license(self, attr, origin=None):
        # Sample attr dict:
        #  'kind': 'bazel-contrib.supply-chain.attribute.license',
        #  'label': '@@+_repo_rules+zlib//:license',
        #  'license_kinds': [{'identifier': '@rules_license//licenses/spdx:Zlib', 'name': 'zlib License'}],
        #  'text': 'external/+_repo_rules+zlib/LICENSE'}
        origin = origin or attr["label"]
        with open(attr["text"]) as license_text:
            text = license_text.read()
            kinds = "+".join([k["identifier"] for k in attr.get("license_kinds", [])])
            self.licenses[origin] = (kinds, text[:50])


def main():
    parser = argparse.ArgumentParser(description="Helper for gathering third party license information.")
    parser.add_argument("--target", help="The target we are processing.")
    parser.add_argument("--output", required=True, help="The output file, mandatory")
    parser.add_argument("--usage_map", required=True, help="The changes output file, mandatory.")
    parser.add_argument("--metadata", action='append', help="path to a metadata bundle")
    options = parser.parse_args()

    attrs = AttrUsage()
    with open(options.usage_map) as inp:
        attrs.load(json.load(inp))

    processed_files = set()  # the wrapper may duplicate things, so dedup here
    for metadata in options.metadata:
        if metadata not in processed_files:
            processed_files.add(metadata)
            attrs.process_package_metadata(metadata)

    # TODO: Do something useful here. That is for a future PR after we get the
    # infrastructure in place.

    with open(options.output, "w") as out:
        out.write(str(attrs.licenses))


if __name__ == '__main__':
    main()

"""Helper tool for gathering third party license information.

NOTE: This is just a shell of the tool to deal with licenses. The behaviors
      and conditions will be documented later as we learn the requirements.

The goal is to turn license data into the format for LICENSES.csv.
Sample:
   Component,Origin,License,Copyright
   core,cel.dev/expr,Apache-2.0,TristonianJones <tswadell@google.com>
   core,cloud.google.com/go/auth,Apache-2.0,Copyright 2019 Google LLC

This change refactors the original helper for correctness.
It now emits something close.

  core,@@+_repo_rules+libffi//:license,@rules_license+//licenses/spdx:GPL-2.0,libffi - Copyright (c) 1996-2024  Anthony Green, R
  core,@@+_repo_rules+xz//:license,@rules_license+//licenses/spdx:0BSD,Permission to use, copy, modify, and/or distribute
  core,@@+_repo_rules+zlib//:license,@rules_license+//licenses/spdx:Zlib,Copyright notice:

Next steps for licenses.csv:
- replace @@+_repo_rules+libffi with the purl of the rule
- remove @rules_license+//licenses in a generic way.
- use the license parser to extact the copyright.

TODO:
- We can recognize ship_source_offer, but don't do anything with it yet.
- figure out how we will merge into the workflow in tasks/license.py
- See if we can have this produce a unified output that could be untarred
  in /opt/datadog/LICENSES so that we don't need pkg_install from each
  library.
"""

import argparse
import csv
import json

_DEBUG = 0

from tasks.licenses import find_copyright_in_text


class AttrUsage:
    def __init__(self):
        self.licenses = {}
        self.attribute_kinds = {}

    def set_kinds(self, attribute_kinds):
        self.attribute_kinds = attribute_kinds

    def process_file(self, file, users):
        kind = self.attribute_kinds.get(file)
        if not kind:
            raise ValueError(f"Got file path that has no kind: '{file}'")
        with open(file) as attr_inp:
            if kind == "build.bazel.rules_license.license":
                self.process_license(json.load(attr_inp))
            else:
                # TODO: When we start writing other types than JSON, we have to be more careful about this
                self.process_attribute_json(file, json.load(attr_inp))

    def process_attribute_json(self, file, attr):
        """Process a standalone attributes file."""
        if _DEBUG > 0:
            print(f"=== {file}")
        kind = attr.get('kind') or None
        if not kind:
            raise ValueError(f"Badly formated attribute file: {file}\n{attr}")

        if kind == "bazel-contrib.supply-chain.attribute.license":
            self.process_license(attr)
            return
        # For now, log unknown things. In the future we can just gracefully ignore them.
        print(f"Warning: Unhandled attribute type: {kind}")

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
            # TODO: We should use the identifier for more precision, but we can't do that until we
            # rebuild the downstream processing of LICENSES.CSV
            kinds = "+".join([k["identifier"] for k in attr.get("license_kinds", [])])
            self.licenses[origin] = (kinds, text[:50])


def main():
    parser = argparse.ArgumentParser(description="Helper for gathering third party license information.")
    parser.add_argument("--target", help="The target we are processing.")
    parser.add_argument("--output", required=True, help="The output file, mandatory")
    parser.add_argument("--usage_map", required=True, help="The changes output file, mandatory.")
    parser.add_argument("--kinds", required=True, help="JSON file mapping file paths to their kinds, mandatory.")
    parser.add_argument("--metadata", action='append', help="path to a metadata bundle")
    options = parser.parse_args()

    attrs = AttrUsage()

    # Load up the map that tells us the type of each input file.
    with open(options.kinds) as inp:
        attrs.set_kinds(json.load(inp))

    with open(options.usage_map) as inp:
        usage = json.load(inp)
    for attr_file, users in usage.items():
        attrs.process_file(attr_file, users)

    processed_files = set()  # the wrapper may duplicate things, so dedup here
    for metadata in options.metadata or []:
        if metadata not in processed_files:
            processed_files.add(metadata)
            attrs.process_package_metadata(metadata)

    licenses = []
    for license_target, data in attrs.licenses.items():
        # Sample: '@@+_repo_rules+libffi//:license': ('@rules_license+//licenses/spdx:GPL-2.0', "libffi - Copyright (c) 1996-2024  ...)
        origin = license_target
        license_type = data[0]
        license_text = data[1]
        copyright = find_copyright_in_text(license_text.split("\n")) or ""
        licenses.append(["core", origin, license_type, copyright])

    with open(options.output, "w", newline="") as out:
        csv_writer = csv.writer(out, quotechar="\"", quoting=csv.QUOTE_MINIMAL)
        for license in licenses:
            csv_writer.writerow(license)


if __name__ == '__main__':
    main()

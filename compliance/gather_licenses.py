"""Helper tool for gathering third party license information.

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

Next steps:
- replace @@+_repo_rules+libffi with the purl of the rule
- remove @rules_license+//licenses in a generic way.
- use the license parser to extact the copyright.
"""

import argparse
import json


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
                self.process_license(file, json.load(attr_inp), users)
            else:
                # TODO: When we start wrtigin other types than JSON, we have to be more careful about this
                self.process_attribute_json(file, json.load(attr_inp), users)

    def process_license(self, file, attr, users):
        # Sample: {
        #  'kind': 'bazel-contrib.supply-chain.attribute.license',
        #  'label': '@@+_repo_rules+zlib//:license',
        #  'license_kinds': [{'identifier': '@rules_license//licenses/spdx:Zlib', 'name': 'zlib License'}],
        #  'text': 'external/+_repo_rules+zlib/LICENSE'}
        with open(attr["text"]) as license_text:
            text = license_text.read()
            # TODO: We should use the identifier for more precision, but we can't do that until we
            # rebuild the downstream processing of LICENSES.CSV
            kinds = ", ".join([k["name"] for k in attr.get("license_kinds", [])])
            self.licenses[attr["label"]] = (kinds, text)

    def process_attribute_json(self, file, attr, users):
        print("=== ", file)
        kind = attr.get('kind') or None
        if not kind:
            raise ValueError(f"Badly formated attribute file: {file}\n{attr}")
        if kind == "bazel-contrib.supply-chain.attribute.license":
            self.process_license(file, attr, users)
        print(f"Unknown attribute type: {kind}")


def main():
    parser = argparse.ArgumentParser(description="Helper for gathering third party license information.")
    parser.add_argument("--target", help="The target we are processing.")
    parser.add_argument("--output", required=True, help="The output file, mandatory")
    parser.add_argument("--usage_map", required=True, help="The changes output file, mandatory.")
    parser.add_argument("--kinds", required=True, help="JSON file mapping file paths to their kinds, mandatory.")
    parser.add_argument("--metadata", action='append', help="path to a metadata bundle")
    options = parser.parse_args()

    attrs = AttrUsage()

    # Load up the map that tells us the type of eac input file.
    with open(options.kinds) as inp:
        attrs.set_kinds(json.load(inp))

    with open(options.usage_map) as inp:
        usage = json.load(inp)
    for attr_file, users in usage.items():
        attrs.process_file(attr_file, users)

    csv = []
    for license_target, data in attrs.licenses.items():
        # Sample: '@@+_repo_rules+libffi//:license': ('@rules_license+//licenses/spdx:GPL-2.0', "libffi - Copyright (c) 1996-2024  ...)
        origin = license_target
        license_type = data[0]
        license_text = data[1]
        short = license_text[:50]  # just print a little to show we have it working
        csv.append(f"core,{origin},{license_type},{short}")

    with open(options.output, "w") as out:
        out.write("\n".join(csv))


if __name__ == '__main__':
    main()

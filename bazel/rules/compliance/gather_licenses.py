"""Helper tool for gathering third party license information.

- Private helper to licenses_csv.bzl
- Cracks open metadata attribute files and deals with them each.
- The behaviors and conditions to be documented later as we learn the requirements.
"""

import argparse
import json


class AttrUsage:
    def __init__(self):
        self.foo = 0
        self.licenses = {}

    def load(self, usage_map):
        for file, users in usage_map.items():
            with open(file) as attr_inp:
                self.process_attribute_json(file, json.load(attr_inp))

    def process_attribute_json(self, file, attr):
        print("=== ", file)
        kind = attr.get('kind') or None
        if not kind:
            raise ValueError(f"Badly formated attribute file: {file}\n{content}")

        if kind == "bazel-contrib.supply-chain.attribute.license":
            # Sample: {
            #  'kind': 'bazel-contrib.supply-chain.attribute.license',
            #  'label': '@@+_repo_rules+zlib//:license',
            #  'license_kinds': [{'identifier': '@rules_license//licenses/spdx:Zlib', 'name': 'zlib License'}],
            #  'text': 'external/+_repo_rules+zlib/LICENSE'}
            with open(attr["text"]) as license_text:
                text = license_text.read()
                kinds = ", ".join([k["identifier"] for k in attr.get("license_kinds", [])])
                self.licenses[attr["label"]] = (kinds, text)
            return

        print("Unknown attribute type: %s" % kind)


def main():
    parser = argparse.ArgumentParser(description="Helper for gathering third party license information.")
    parser.add_argument("--target", help="The target we are processing.")
    parser.add_argument("--output", required=True, help="The output file, mandatory")
    parser.add_argument("--usage_map", required=True, help="The changes output file, mandatory.")
    parser.add_argument("--metadata", action='append', help="path to a metadata bundle")
    parser.add_argument("--purl", action='append', help="the PURL for this metadata")
    options = parser.parse_args()

    attrs = AttrUsage()
    with open(options.usage_map) as inp:
        attrs.load(json.load(inp))

    # TODO: Do something useful here. That is for a future PR after we get the
    # infrastructure in place.

    with open(options.output, "w") as out:
        out.write(str(attrs.licenses))


if __name__ == '__main__':
    main()

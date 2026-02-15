"""Helper tool for gathering third party license information.

This gathers package metadata annotations and emits packages them the
way we need for the DataDog Agent. That includes several things.

1.  Create a section for LICENSES-3rdparty.csv.
Sample:
   Component,Origin,License,Copyright
   core,cel.dev/expr,Apache-2.0,TristonianJones <tswadell@google.com>
   core,cloud.google.com/go/auth,Apache-2.0,Copyright 2019 Google LLC
   core,@@+_repo_rules+libffi//:license,@rules_license+//licenses/spdx:GPL-2.0,libffi - Copyright (c) 1996-2024  Anthony Green, R
   core,@@+_repo_rules+xz//:license,@rules_license+//licenses/spdx:0BSD,Permission to use, copy, modify, and/or distribute
   core,@@+_repo_rules+zlib//:license,@rules_license+//licenses/spdx:Zlib,Copyright notice:

STATUS: In progress

Next steps for licenses.csv:
- replace @@+_repo_rules+libffi with the purl of the rule
- remove @rules_license+//licenses in a generic way.
- use the license parser to extact the copyright.


2. Produce "offer to ship source" files.

E.g. /opt/datadog-agent/sources/libxcrypt/offer.txt
Content:
   This package ships libxcrypt.
   Contact Datadog Support (http://docs.datadoghq.com/help/) to request the sources of this software."

We can do better by putting them all a single file.


3. Copy license files to /opt/datadog-agent/LICENSES

Format:  <package_name>-<license_file_name>

STATUS: Not started

4. Produce stanzas for /opt/datadog-agent/LICENSE

These are appended to our overall license text.

Sample:
  This product bundles the third-party transitive dependency sigs.k8s.io/yaml/goyaml.v3,
  which is available under a "MIT" License.

It currently looks like we skip a lot of C code, so this is a chance to fix that.

STATUS: Not started
"""

import argparse
import csv
import json
import os

from tasks.licenses import find_copyright_in_text

_DEBUG = 0


def repo_of_target(target):
    split = target.find("//")
    if split <= 0:
        return ""
    return target[:split]


def origin_to_module(origin):
    """Turn the origin of the license back to a module name.

    Args:
        origin: str, maybe a bazel target, maybe a purl
    Returns:
        A name
    """
    if origin.startswith("pkg:"):
        return purl_to_package(origin)

    # Now we might see any of these forms
    #  @+_repo_rules+bzip2//:license
    #  @rules_flex+//:license
    #  @@//third_party/some/package:license
    #  //deps/package:license

    # It is a label, get rid of the target name.
    colon = origin.rfind(":")
    if colon > 0:
        origin = origin[0:colon]
    # and now that everything after the colon is gone, we
    # have new cruft at the end to strip.
    origin = origin.rstrip("+/")
    # and the cruft at the start
    origin = origin.lstrip("@")

    # Is the license in our repo? Like this? @@//deps/msodbcsql18
    if origin.startswith("//"):
        # All we need is the package
        package = origin.split("/")[-1]
        return package

    # Now we hit the bzlmod ugly names, +_repo_rules+bzip2 or rules_flex
    # Take the word after the last +.
    plus = origin.rfind("+")
    if plus >= 0:
        remainder = origin[plus + 1 :]
        if remainder:
            return remainder
        # If there's nothing after the +, strip it and continue
        origin = origin[:plus]
    return origin


def purl_to_package(purl):
    # pkg:https/download.savannah.nongnu.org/attr@2.5.1?url=https://download.savannah.nongnu.org/releases/attr/attr-%2.5.1.tar.xz
    base = purl.split("?")[0]
    package_version = base.split("/")[-1]
    package = package_version.split("@")[0]
    return package


class AttrUsage:
    def __init__(self):
        self.licenses = {}
        self.attribute_kinds = {}
        self.offers = {}

    def set_kinds(self, attribute_kinds):
        self.attribute_kinds = attribute_kinds

    def process_file(self, file, users):
        kind = self.attribute_kinds.get(file)
        if not kind:
            raise ValueError(f"Got file path that has no kind: '{file}'")
        with open(file, encoding="UTF-8") as attr_inp:
            if kind == "build.bazel.rules_license.license":
                self.process_license(attr=json.load(attr_inp))
            else:
                # TODO: When we start writing other types than JSON, we have to be more careful about this
                self.process_attribute_json(file, attr=json.load(attr_inp))

    def process_attribute_json(self, file, attr):
        """Process a standalone attributes file."""
        if _DEBUG > 0:
            print(f"=== {file}")
        kind = attr.get('kind') or None
        if not kind:
            raise ValueError(f"Badly formated attribute file: {file}\n{attr}")

        if kind == "bazel-contrib.supply-chain.attribute.license":
            self.process_license(attr=attr)
            return
        if kind == "datadog.agent.attribute.ship_source_offer":
            # safe to ignore, we get it via process_package_metadata.
            return
        # For now, log unknown things. In the future we can just gracefully ignore them.
        print(f"Warning: Unhandled attribute type: {kind}")

    def process_package_metadata(self, package_metadata_file):
        """Process a package_metadata bundle file."""
        with open(package_metadata_file, encoding="UTF-8") as inp:
            mx = json.load(inp)
            purl = mx.get("purl") or "unset"
            if _DEBUG > 0:
                print(json.dumps(mx, indent="  "))
                print(f"  metadata target: {mx['label']}")
                print(f"  PURL: {purl}")
            if mx.get("attributes"):
                for attr_type, file in mx["attributes"].items():
                    if attr_type == "build.bazel.rules_license.license":
                        with open(file, encoding="UTF-8") as attr_inp:
                            self.process_license(purl=purl, attr=json.load(attr_inp))
                    elif attr_type == "datadog.agent.attribute.ship_source_offer":
                        self.process_source_offer(purl=purl)
                    else:
                        print(f"Warning: Unhandled attribute type: {attr_type}, file: {file}")

    def process_license(self, purl=None, attr=None):
        # Sample attr dict:
        #  'kind': 'bazel-contrib.supply-chain.attribute.license',
        #  'label': '@@+_repo_rules+zlib//:license',
        #  'license_kinds': [{'identifier': '@rules_license//licenses/spdx:Zlib', 'name': 'zlib License'}],
        #  'text': 'external/+_repo_rules+zlib/LICENSE'}
        origin = purl or attr["label"]
        with open(attr["text"], "rb") as license_text:
            text = license_text.read()
            # TODO: We should use the identifier for more precision, but we can't do that until we
            # rebuild the downstream processing of LICENSES.CSV
            kinds = "+".join([k["identifier"] for k in attr.get("license_kinds", [])])
            self.licenses[origin] = (kinds, text, attr["text"])

    def process_source_offer(self, purl=None):
        package = (purl.split("?")[0]).split("/")[-1]
        self.offers[package] = (
            f"This package ships {package}\nContact Datadog Support (http://docs.datadoghq.com/help/) to request the sources of this software.\n"
        )


def main():
    parser = argparse.ArgumentParser(description="Helper for gathering third party license information.")
    parser.add_argument("--target", help="The target we are processing.")
    parser.add_argument("--csv_out", help="The CSV style output file.")
    parser.add_argument("--licenses_dir", help="Directory to copy license texts to.")
    parser.add_argument("--offers_dir", help="Directory to write 'ship source' offers to.")
    parser.add_argument("--offers_out", help="File to write 'ship source' offers to.")
    parser.add_argument("--usage_map", required=True, help="The changes output file, mandatory.")
    parser.add_argument("--kinds", required=True, help="JSON file mapping file paths to their kinds, mandatory.")
    parser.add_argument("--metadata", action='append', help="path to a metadata bundle")
    options = parser.parse_args()

    attrs = AttrUsage()

    # Load up the map that tells us the type of each input file.
    with open(options.kinds, encoding="UTF-8") as inp:
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
        # The license text is just bytes, not always UTF-8, so be forgiving.
        license_text = data[1].decode(encoding="utf-8", errors="ignore")
        copyright = find_copyright_in_text(license_text.split("\n")) or ""
        licenses.append(["core", origin, license_type, copyright])

    if options.csv_out:
        with open(options.csv_out, "w", newline="", encoding="UTF-8") as out:
            csv_writer = csv.writer(out, quotechar="\"", quoting=csv.QUOTE_MINIMAL)
            for license in licenses:
                csv_writer.writerow(license)

    if options.offers_dir:
        for package in attrs.offers:
            short_package = package.split("@")[0]  #  foo@version => foo
            os.mkdir(os.path.join(options.offers_dir, short_package))
            copy_to = os.path.join(options.offers_dir, short_package, "offer.txt")
            if _DEBUG > 1:
                print(f"writing {copy_to}")
            with open(copy_to, "w", encoding="UTF-8") as out:
                out.write(attrs.offers[package])

    if options.offers_out:
        with open(options.offers_out, "w", encoding="UTF-8") as out:
            out.write("\n".join(attrs.offers.values()))

    if options.licenses_dir:
        for origin, data in attrs.licenses.items():
            canonical_package = origin_to_module(origin)
            if _DEBUG > 2:
                print(f"{origin} => {canonical_package}")
            # This good enough for today. The DD policy is that all licenses have SPDX
            # identifiers, so the repo name is always constant.
            license_type = data[0].replace("@rules_license+//licenses/spdx:", "")
            license_text = data[1]
            license_file = data[2]
            license_file_short = os.path.basename(license_file)
            copy_to = os.path.join(options.licenses_dir, f"{canonical_package}-{license_file_short}")
            if _DEBUG > 1:
                print(f"writing for {canonical_package} to {copy_to}")
            with open(copy_to, "wb") as out:
                out.write(license_text)


if __name__ == '__main__':
    main()

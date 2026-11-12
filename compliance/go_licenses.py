"""Match the Go packages gathered by go_third_party_licenses to LICENSE-3rdparty.csv.

Reads the JSON list of {importpath, label, purl} written by the rule, looks up each importpath
in the Origin column of the CSV, and writes the distinct license ids plus a per-package CSV.
Packages under a `.wwhrd.yml` exception are dropped, as `dda inv generate-licenses` does. Any
other package missing from the CSV is an error.
"""

import argparse
import csv
import json
import sys

import yaml


def load_licenses(csv_path):
    """Returns {origin: license} from a LICENSE-3rdparty.csv style file."""
    with open(csv_path, encoding="utf-8") as f:
        return {row["Origin"]: row["License"] for row in csv.DictReader(f)}


def load_exceptions(wwhrd_path):
    with open(wwhrd_path, encoding="utf-8") as f:
        return (yaml.safe_load(f) or {}).get("exceptions") or []


def is_excepted(importpath, exceptions):
    """Applies wwhrd exception patterns, where a trailing `/...` means the path and below."""
    for pattern in exceptions:
        if pattern.endswith("/..."):
            prefix = pattern[: -len("/...")]
            if importpath == prefix or importpath.startswith(prefix + "/"):
                return True
        elif importpath == pattern:
            return True
    return False


def classify(packages, licenses, exceptions):
    """Splits packages into (matched, unmatched).

    Args:
        packages: list of {importpath, label, purl} dicts, possibly with duplicates.
        licenses: {origin: license}.
        exceptions: wwhrd exception patterns.

    Returns:
        matched: sorted list of (importpath, purl, license).
        unmatched: sorted list of (importpath, label).
    """
    matched = {}
    unmatched = {}
    for package in packages:
        importpath = package["importpath"]
        if importpath in matched or is_excepted(importpath, exceptions):
            continue
        license_id = licenses.get(importpath)
        if license_id:
            matched[importpath] = (importpath, package.get("purl", ""), license_id)
            unmatched.pop(importpath, None)
        else:
            unmatched.setdefault(importpath, (importpath, package.get("label", "")))
    return sorted(matched.values()), sorted(unmatched.values())


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--packages", required=True)
    parser.add_argument("--csv", required=True)
    parser.add_argument("--wwhrd", required=True)
    parser.add_argument("--license_ids_out", required=True)
    parser.add_argument("--packages_out", required=True)
    args = parser.parse_args(argv)

    with open(args.packages, encoding="utf-8") as f:
        packages = json.load(f)
    matched, unmatched = classify(packages, load_licenses(args.csv), load_exceptions(args.wwhrd))
    if unmatched:
        print(
            f"{len(unmatched)} Go package(s) are not in {args.csv} and not excepted in {args.wwhrd}.\n"
            "Run `dda inv generate-licenses`, or add an exception:",
            file=sys.stderr,
        )
        for importpath, label in unmatched:
            print(f"  {importpath} ({label})", file=sys.stderr)
        return 1

    with open(args.license_ids_out, "w", encoding="utf-8") as f:
        for license_id in sorted({license_id for _, _, license_id in matched}):
            f.write(license_id + "\n")
    with open(args.packages_out, "w", encoding="utf-8", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["importpath", "purl", "license"])
        writer.writerows(matched)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

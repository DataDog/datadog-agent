"""Declares rule `ship_source_offer`.

This is a package_metadata attribute that declares we must offer to ship
the source to the product if this package is used. The build process
will include a mention in the file offer.txt if it is used in the
artifact we are building.

Usage:

```
load("@package_metadata//rules:package_metadata.bzl", "package_metadata")
load("@@//compliance/rules:ship_source_offer.bzl", "ship_source_offer")

package(default_package_metadata = [":package_metadata", ":ship_source_offer"])

package_metadata(
    name = "package_metadata",
    attributes = [
        ":ship_source_offer",
        ...
    ],
    purl = PURL,
)

ship_source_offer(name = "ship_source_offer")
```

It seems unusual to put and attribute like :ship_source_offer in the
package default rather than the package_metadata declaration. The
reason for doing so is a combination.
- Aspect traversal hits the top level package_metadata target and stops,
  so that the aspect never sees the attributes directly. This is a bazel issue.
- PackageMetadataInfo does not contain the kinds of the attributes, we
  can't trigger on a special case one without reading the metadata data
  file itself.
- Reading the file pushes detection from analysis time into execution,
  making it harder to trigger new things at build time.

We're working through opinions on the second point in the supply_chain project.
The possible resolutions are:
- Add the kinds to the provider in the core rule set
- Add the kinds to the provider in our local fork. This is an expected path
  for companies with special constraints.
- Fix bazel
- Push handling of ship_source_info down to execution phase.

For now, the recommendation is to use the redundant declaration shown
above, so that either way the rules change, we'll be covered without
having to migrate first.
"""

load("@package_metadata//providers:package_attribute_info.bzl", "PackageAttributeInfo")

# This must be public because we include it from external modules. There is no way to
# represent that within the visibility syntax.
visibility("public")

SHIP_SOURCE_ATTR_KIND = "datadog.agent.attribute.ship_source_offer"

def _init_ssi():
    return {}

ShipSourceOfferInfo, _create = provider(
    doc = "Indicate we must include an offer to ship our source if this pacakge is used.",
    fields = {},
    init = _init_ssi,
)

def _ship_source_offer_impl(ctx):
    attribute = {
        "kind": SHIP_SOURCE_ATTR_KIND,
        "label": str(ctx.label),
    }
    output = ctx.actions.declare_file("_{}_attribute.json".format(ctx.attr.name))
    ctx.actions.write(
        output = output,
        content = json.encode(attribute),
    )
    files = depset(direct = [output])
    return [
        DefaultInfo(files = files),
        PackageAttributeInfo(
            kind = SHIP_SOURCE_ATTR_KIND,
            attributes = output,
        ),
        ShipSourceOfferInfo(),
    ]

_ship_source_offer = rule(
    implementation = _ship_source_offer_impl,
    doc = """Declares that we must offer to ship the source of our code if this package is used.""",
    provides = [
        PackageAttributeInfo,
        ShipSourceOfferInfo,
    ],
)

def ship_source_offer(
        *,  # Disallow unnamed attributes.
        name,
        # Common attributes (subset since this target is non-configurable).
        visibility = None):
    _ship_source_offer(
        name = name,
        visibility = visibility,
        package_metadata = [],
    )

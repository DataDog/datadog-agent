"""Declares rule `ship_source_offer`.

This is a package_metadata attribute that declares we must offer to ship
the souce to the product if this package is used. The build process
will include the a mention in the file offer.txt if it is used in the
artifact we are building.
"""

load("@package_metadata//providers:package_attribute_info.bzl", "PackageAttributeInfo")

# This must be public because we include it from external modules. There is no way to
# represent that within the visibility syntax.
visibility("public")

KIND = "datadog.agent.attribute.ship_source_offer"

def _ship_source_offer_impl(ctx):
    attribute = {
        "kind": KIND,
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
            kind = KIND,
            attributes = output,
        ),
    ]

_ship_source_offer = rule(
    implementation = _ship_source_offer_impl,
    doc = """Declares that we must offer to ship the source of our code if this package is used.""",
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

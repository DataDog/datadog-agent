"""Gather licenses used by bazel targets into the format we use for reporting."""

load(
    "@package_metadata//:defs.bzl",
    "PackageAttributeInfo",
    "PackageMetadataInfo",
)
load(
    "@supply_chain_tools//gather_metadata:core.bzl",
    "gather_metadata_info_common",
    "should_traverse",
)
load(
    "@supply_chain_tools//gather_metadata:providers.bzl",
    "TransitiveMetadataInfo",
    "null_transitive_metadata_info",
)

DEBUG_LEVEL = 0

def update_attribute_to_consumers(attribute_to_consumers, file, target):
    """Maintains map of metadata attribute files to the targets using them.

    Args:
        attribute_to_consumers: (in/out) the map.
        file: attribute file object.
        target: a target.
    """
    if file not in attribute_to_consumers:
        attribute_to_consumers[file] = []
    attribute_to_consumers[file].append(str(target))

# All top level metadata processing rules will generally wrap gahter with their
# Own aspect to walk the tree. This wrapper is usually not much different than
# the example here. The variation is usually only to provide the set of
# providers we want to collect. This allows for organization specific providers
# to be gathered in the same pass as the canonical ones from suppply_chain.
#
def _gather_metadata_info_impl(target, ctx):
    return gather_metadata_info_common(
        target,
        ctx,
        want_providers = [PackageAttributeInfo, PackageMetadataInfo],
        provider_factory = TransitiveMetadataInfo,
        null_provider_instance = null_transitive_metadata_info,
        filter_func = should_traverse,
    )

gather_metadata_info = aspect(
    doc = """Collects metadata providers into a single TransitiveMetadataInfo provider.""",
    implementation = _gather_metadata_info_impl,
    attr_aspects = ["*"],
    provides = [TransitiveMetadataInfo],
    apply_to_generating_rules = True,
)

def _handle_attribute_provider(
        metadata_provider,
        target = None,
        args = None,
        inputs = None,
        report = None,
        attribute_to_consumers = None):
    """Handle an individual metadata attribute provider.

    Args:
        metadata_provider: A provider instance
        target: target to which this attribute applies
        args: (in/out) list of command line args we are building
        inputs: (in/out) list of files needed for that command line
        report: (in/out) list of things we want to say to the user.
                This is for illustrating how to use these rules, and
                is not needed for the SBOM.
        attribute_to_consumers: Map of attribute providers back to the packages that use them.
    """

    # We are presuming having metadata means you are a PackageMetadataInfo
    # instead of a PackageAttributeInfo.
    if hasattr(metadata_provider, "metadata"):
        # We should add a kind into the file itself and treat these like
        # other attributes. That requires upstream changes.
        args.add("-metadata %s" % metadata_provider.metadata.path)
        inputs.extend(metadata_provider.files.to_list())
        return

    # Anything else we encounter should be PackageAttributeInfo. That has 3
    # elements: kind: str, attributes: dict, files: List[File]
    # But also, you can make org specific custom types, as long as you use the
    # kind to distingish them if they need special processing.
    kind = metadata_provider.kind if hasattr(metadata_provider, "kind") else "_UNKNOWN_"
    if DEBUG_LEVEL > 1:
        # buildifier: disable=print
        print("##-- %s: %s" % (kind, str(metadata_provider)))
    if not kind:
        return

    if hasattr(metadata_provider, "attributes"):
        update_attribute_to_consumers(attribute_to_consumers, metadata_provider.attributes, target)

        report.append("  Attribute data: %s" % metadata_provider.attributes.short_path)
        if hasattr(metadata_provider, "files"):
            inputs.extend(metadata_provider.files.to_list())
            for f in metadata_provider.files.to_list():
                report.append("    file: %s" % f.path)

    # Check for extras.
    # This is for debugging during early development. There should be no
    # extra fields.
    for field in sorted(dir(metadata_provider)):
        if field in ("attributes", "files", "kind"):
            continue
        value = getattr(metadata_provider, field)
        report.append("Extra field: %s: %s" % (field, value))

def _handle_transitive_collector(t_m_i, args, inputs, report, attribute_to_consumers):
    """Process a TransitiveMetadataInfo.

    Args:
        t_m_i: A provider instance
        args: (in/out) list of command line args we are building
        inputs: (in/out) list of files needed for that command line
        report: (in/out) list of things we want to say to the user.
                This is for illustrating how to use these rules, and
                is not needed for the SBOM.
        attribute_to_consumers: Map of attribute providers back to the
                 packages that use them.
    """
    if hasattr(t_m_i, "metadata"):
        report.append("Target %s" % t_m_i.target)

        for metadata in t_m_i.metadata.to_list():
            _handle_attribute_provider(
                metadata,
                target = t_m_i.target,
                args = args,
                inputs = inputs,
                report = report,
                attribute_to_consumers = attribute_to_consumers,
            )
        if hasattr(t_m_i, "trans"):
            fail("TransititiveMetadataInfo contains both metadata and trans." + str(t_m_i))

def _license_csv_impl(ctx):
    # Gather all metadata and make a report from that

    # TODO: Replace this
    # The code below just dumps the collected metadata providers in a somewhat
    # pretty printed way.  In reality, we need to read the files associated with
    # each attribute to get the real data. So this should be a rule to pass
    # all the files to a helper which generates a formated report.
    # That is clearly a job for another day.

    if TransitiveMetadataInfo not in ctx.attr.target:
        fail("Missing metadata for %s" % ctx.attr.target)
    t_m_i = ctx.attr.target[TransitiveMetadataInfo]
    if DEBUG_LEVEL > 1:
        # buildifier: disable=print
        print(t_m_i)

    inputs = []
    report = []
    attribute_to_consumers = {}

    args = ctx.actions.args()
    args.add("--output", ctx.outputs.out.path)

    report.append("Top label: %s" % str(ctx.attr.target.label))
    if hasattr(t_m_i, "target"):
        report.append("Target: %s" % str(t_m_i.target))
        args.add("--target '%s'" % str(t_m_i.target))

    # It is possible for the top level target to have metadata, but rare.
    if hasattr(t_m_i, "metadata"):
        if DEBUG_LEVEL > 0:
            # buildifier: disable=print
            print("TOP HAS DIRECTS")
        for direct in t_m_i.metadata.to_list():
            _handle_attribute_provider(
                metadata = direct,
                target = t_m_i.target,
                args = args,
                inputs = inputs,
                report = report,
                attribute_to_consumers = attribute_to_consumers,
            )

    if hasattr(t_m_i, "trans"):
        for trans in t_m_i.trans.to_list():
            _handle_transitive_collector(trans, args, inputs, report, attribute_to_consumers)

    # For the next few months of co-development with supply-chain, print a
    # report of what we have. It's not the final output. It just helps see what
    # we have.
    # buildifier: disable=print
    print("Report: \n   %s\n" % "\n   ".join(report))

    # Now do the work of turning this into something good.

    # First, dump the map of attributes to the targets to which they apply.
    # That drives the processor.  Normally, we make up an undeclared temp file,
    # but we can declare a named output for this that a user could specify.
    # That would be used in a macro where we may want to drive several optional
    # processes off the gathered aspect data, without running it multiple times.
    if not ctx.outputs.usage_map_private:
        map_file = ctx.actions.declare_file("_%s_map.json" % ctx.label.name)
    else:
        map_file = ctx.outputs.usage_map_private
    inputs.append(map_file)
    usage_map = {key.path: value for key, value in attribute_to_consumers.items()}
    if DEBUG_LEVEL > 1:
        # buildifier: disable=print
        print(json.encode_indent(usage_map))
    ctx.actions.write(map_file, json.encode_indent(usage_map, indent = " "))
    args.add("--usage_map", map_file.path)

    ctx.actions.run(
        mnemonic = "GatherLicenses",
        progress_message = "Writing: %s" % ctx.outputs.out.path,
        # inputs = inputs,
        inputs = inputs,
        executable = ctx.executable._processor,
        arguments = [args],
        outputs = [ctx.outputs.out],
        env = {
            "LANG": "en_US.UTF-8",
            "LC_CTYPE": "UTF-8",
            "PYTHONIOENCODING": "UTF-8",
            "PYTHONUTF8": "1",
        },
        use_default_shell_env = True,
    )
    return [DefaultInfo(files = depset([ctx.outputs.out]))]

license_csv = rule(
    implementation = _license_csv_impl,
    doc = """Create a csv format list of the licenses used by a target.""",
    attrs = {
        "target": attr.label(
            doc = """Targets to gather licenses for.""",
            aspects = [gather_metadata_info],
        ),
        "out": attr.output(
            doc = """Output file.""",
            mandatory = True,
        ),
        "usage_map_private": attr.output(
            doc = """Intermediate dump of data to drive gather_licenses. Private.""",
            mandatory = False,
        ),
        "_processor": attr.label(
            doc = """processor to read individual atttributes and turn into the CSV file.""",
            default = Label("//bazel/rules/compliance:gather_licenses"),
            cfg = "exec",
            executable = True,
            allow_files = True,
        ),
    },
)

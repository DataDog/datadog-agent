"""Graph-only metadata serialization.

This module provides functions to serialize TransitiveMetadataInfo providers
to JSON in a graph-only format that avoids duplicating attributes already
present in per-package metadata files. The aggregate JSON contains only the
dependency graph structure (nodes + edges), while attributes remain in
individual metadata files as the single source of truth.
"""

load(":providers.bzl", "TransitiveMetadataInfo")

def _label_to_string(label):
    """Convert a Label to canonical string representation.

    For main workspace targets, builds a canonical string from Label properties
    to ensure consistent output across Bazel versions (5.x uses @//, 6.x uses @@//).
    For external workspaces, uses str() directly.

    Args:
        label: A Label object

    Returns:
        Canonical string: "//package:name" or "@repo//package:name"
    """
    if not label.workspace_name:
        # Main workspace: build from properties to avoid @ prefix
        parts = ["//"]
        if label.package:
            parts.append(label.package)
        parts.append(":")
        parts.append(label.name)
        return "".join(parts)
    # External workspace: use str() which includes @workspace//
    return str(label)

def _get_metadata_file_path(target_with_metadata_info):
    """Extract metadata file path from providers.

    Args:
        target_with_metadata_info: TargetWithMetadataInfo provider

    Returns:
        String path to the .package-metadata.json file, or empty string if not found
    """
    for provider in target_with_metadata_info.metadata.to_list():
        # PackageMetadataInfo has a 'metadata' field pointing to the JSON file
        if hasattr(provider, "metadata") and hasattr(provider.metadata, "path"):
            return provider.metadata.path
    return ""

def _build_node(target_with_metadata_info):
    """Build a graph node for a target.

    Args:
        target_with_metadata_info: TargetWithMetadataInfo provider

    Returns:
        Dictionary representing a node in the graph
    """
    return {
        "label": _label_to_string(target_with_metadata_info.target),
        "metadata_file": _get_metadata_file_path(target_with_metadata_info),
    }

def _build_edges(all_targets):
    """Build edges from TargetWithMetadataInfo list.

    Args:
        all_targets: List of TargetWithMetadataInfo providers

    Returns:
        List of edge dictionaries
    """
    edges = []
    for target_info in all_targets:
        if hasattr(target_info, "direct_deps") and target_info.direct_deps:
            from_label = _label_to_string(target_info.target)
            for dep in target_info.direct_deps:
                edges.append({
                    "from": from_label,
                    "to": _label_to_string(dep),
                    "type": "depends_on",
                })
    return edges

def metadata_info_to_json(metadata_info):
    """Convert TransitiveMetadataInfo to graph-only JSON.

    This is the main entry point for serialization. It produces JSON with
    only the dependency graph structure (nodes + edges). Attributes are
    NOT duplicated in this output - they remain in per-package metadata files.

    Output format:
    {
      "schema_version": "1.0",
      "root_target": "//some:target",
      "nodes": [
        {
          "label": "//some:target",
          "metadata_file": "path/to/target.package-metadata.json"
        }
      ],
      "edges": [
        {
          "from": "//some:target",
          "to": "//other:dep",
          "type": "depends_on"
        }
      ]
    }

    Args:
        metadata_info: TransitiveMetadataInfo provider

    Returns:
        List containing a single JSON string representation
        (list for backwards compatibility with write_metadata_info)
    """

    # Extract all targets from the transitive depset
    all_targets = metadata_info.transitive.to_list()

    # Build nodes (sorted by label for deterministic output)
    nodes = [_build_node(target_info) for target_info in sorted(all_targets, key = lambda x: str(x.target))]

    # Build edges
    edges = _build_edges(all_targets)

    # Build the output structure
    output = {
        "schema_version": "1.0",
        "root_target": _label_to_string(metadata_info.top_level_target) if metadata_info.top_level_target else "",
        "nodes": nodes,
        "edges": edges,
    }

    # Serialize to JSON using built-in json.encode
    return [json.encode(output)]

def write_metadata_info(ctx, deps, json_out):
    """Writes TransitiveMetadataInfo providers for a set of targets as JSON.

    This function writes graph-only JSON that contains dependency structure
    but not attribute duplication. Per-package metadata files remain the
    single source of truth for attributes.

    Usage:
      write_metadata_info must be called from a rule implementation, where the
      rule has run the gather_metadata_info aspect on its deps to
      collect the transitive closure of metadata providers into a
      TransitiveMetadataInfo provider.

      foo = rule(
        implementation = _foo_impl,
        attrs = {
           "deps": attr.label_list(aspects = [gather_metadata_info])
        }
      )

      def _foo_impl(ctx):
        ...
        out = ctx.actions.declare_file("%s_metadata.json" % ctx.label.name)
        write_metadata_info(ctx, ctx.attr.deps, out)

    Args:
      ctx: context of the caller
      deps: a list of deps which should have TransitiveMetadataInfo providers.
            This requires that you have run the gather_metadata_info
            aspect over them
      json_out: output handle to write the JSON info
    """
    metadata_jsons = []
    for dep in deps:
        if TransitiveMetadataInfo in dep:
            metadata_jsons.extend(metadata_info_to_json(dep[TransitiveMetadataInfo]))

    # Wrap multiple roots in an array
    if len(metadata_jsons) == 1:
        content = metadata_jsons[0]
    else:
        # Use json.encode for array wrapping too
        decoded = [json.decode(j) for j in metadata_jsons]
        content = json.encode(decoded)

    ctx.actions.write(
        output = json_out,
        content = content,
    )

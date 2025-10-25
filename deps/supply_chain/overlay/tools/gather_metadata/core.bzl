"""Rules and macros for collecting package_metadata providers."""

load(":providers.bzl", "TargetWithMetadataInfo", "TransitiveMetadataInfo")
load(":rule_filters.bzl", "rule_to_excluded_attributes")
load(":trace.bzl", "TraceInfo")

DEBUG_LEVEL = 0

def should_traverse(ctx, attr, user_filters = None):
    """Checks if the dependent attribute should be traversed.

    Note for the future: We can vastly inmprove the peformance by
    moving this to a Bazel 9 style aspect traversal filter.

    Args:
      ctx: The aspect evaluation context.
      attr: The name of the attribute to be checked.
      user_filters: Additional dictionary of per-rule attribute filters.

    Returns:
      True iff the attribute should be traversed.
    """
    per_rule_filters = [rule_to_excluded_attributes]
    if user_filters:
        per_rule_filters.append(user_filters)

    for filters in per_rule_filters:
        always_ignored = filters.get("*", [])
        if attr in always_ignored:
            return False
        rule_specific_filter = filters.get(ctx.rule.kind, None)
        if rule_specific_filter:
            if (attr in rule_specific_filter or
                "*" in rule_specific_filter or
                ("_*" in rule_specific_filter and attr.startswith("_"))):
                return False
    return True

def _get_transitive_metadata(
        ctx,
        trans_tmi,
        provider = None,
        null_provider_instance = None,
        filter_func = None,
        traces = None):
    """Gather the collection provider instances of interest from our children.

    This is a helper to pull up the collected metadata info from children so
    that we can rebundle into the next level efficiently. It revolves around
    a "collection provider" which is the transitive collected data so far.
    While this method is intended to be generic, it is only built and tested
    with TransitiveMetadataInfo.

    Args:
        ctx: the ctx
        trans_tmi: (output) list of the depsets in the children
        provider: the transitive collection provider.
        null_provider_instance: a singleton instance of the empty provider.
        filter_func: filter to determine to skip.
        traces: debuging traces
    """
    attrs = [attr for attr in dir(ctx.rule.attr)]
    for name in attrs:
        if filter_func and not filter_func(ctx, name):
            if DEBUG_LEVEL > 2:
                print("Trimming attribute %s of %s" % (name, ctx.rule.kind))
            continue
        if DEBUG_LEVEL > 4:
            print("CHECKING attribute %s of %s" % (name, ctx.rule.kind))

        attr_value = getattr(ctx.rule.attr, name)

        # Make scalers into a lists for convenience.
        if type(attr_value) != type([]):
            attr_value = [attr_value]

        for dep in attr_value:
            # Ignore anything that isn't a target
            if type(dep) != "Target":
                continue

            # Targets can also include things like input files that won't have the
            # aspect, so we additionally check for the aspect rather than assume
            # it's on all targets.  Even some regular targets may be synthetic and
            # not have the aspect. This provides protection against those outlier
            # cases.
            if provider in dep:
                info = dep[provider]
                if hasattr(info, "traces") and getattr(info, "traces"):
                    for trace in info.traces:
                        traces.append("(" + ", ".join([str(ctx.label), ctx.rule.kind, name]) + ") -> " + trace)
                if info != null_provider_instance:
                    trans_tmi.append(info.trans)

def gather_metadata_info_common(
        target,
        ctx,
        want_providers = None,
        provider_factory = None,
        null_provider_instance = None,
        filter_func = None):
    """Collect package metadata info from myself and my deps.

    Any single target might directly depend on a package metadata, or depend on
    something that transitively depends on a package metadata, or neither.
    This aspect bundles all those into a single provider. At each level, we add
    in new direct metadata deps found and forward up the transitive information
    collected so far.

    This is a common abstraction for crawling the dependency graph. It is
    parameterized to allow specifying the provider that is populated with
    results. It is configurable to select only a subset of providers. It
    is also configurable to specify which dependency edges should not
    be traced for the purpose of tracing the graph.

    Args:
      target: The target of the aspect.
      ctx: The aspect evaluation context.
      want_providers: a list of providers of interest
      provider_factory: abstracts the provider returned by this aspect
      null_provider_instance: a singleton instance of the empty provider. Reusing a
          a singleton across a large graph can save significant memory.
      filter_func: a function that returns true IFF the dep edge should be ignored

    Returns:
      provider of parameterized type
    """

    # TODO(aiuto): Consider dropping this hack.
    # A hack until https://github.com/bazelbuild/rules_license/issues/89 is
    # fully resolved. If exec is in the bin_dir path, then the current
    # configuration is probably cfg = exec.
    if "-exec-" in ctx.bin_dir.path:
        return [null_provider_instance or provider_factory()]

    # First we gather my direct metadata providers.
    # This captures the pairs if
    got_providers = []
    package_info = []
    if DEBUG_LEVEL > 1:
        print("==============================================\n %s (%s) \n" % (target.label, ctx.rule.kind))
    if hasattr(ctx.rule.attr, "kind") and ctx.rule.attr.kind == "build.bazel.attribute.license":
        # Don't try to gather licenses from the license rule itself. We'll just
        # blunder into the text file of the license and pick up the default
        # attribute of the package, which we don't want.
        pass
    else:
        if hasattr(ctx.rule.attr, "package_metadata"):
            package_metadata = ctx.rule.attr.package_metadata
        elif hasattr(ctx.rule.attr, "applicable_licenses"):
            package_metadata = ctx.rule.attr.applicable_licenses
        else:
            package_metadata = []
        for metadata_dependency in package_metadata:
            if DEBUG_LEVEL > 1:
                print("checking", metadata_dependency.label)
            for wanted_provider in want_providers:
                if wanted_provider in metadata_dependency:
                    got_providers.append(metadata_dependency[wanted_provider])

    if DEBUG_LEVEL > 0 and got_providers:
        print("  GOT: ", target.label, got_providers)

    # Now gather transitive collection of providers from the children this
    # target depends upon.
    trans_tmi = []
    traces = []
    _get_transitive_metadata(
        ctx = ctx,
        trans_tmi = trans_tmi,
        provider = provider_factory,
        null_provider_instance = null_provider_instance,
        filter_func = filter_func,
        traces = traces,
    )

    # If this is the target, start the sequence of traces.
    if hasattr(ctx.attr, "_trace"):
        if ctx.attr._trace[TraceInfo].trace and ctx.attr._trace[TraceInfo].trace in str(ctx.label):
            traces = [ctx.attr._trace[TraceInfo].trace]

    # Trim the number of traces accumulated since the output can be quite large.
    # A few representative traces are generally sufficient to identify why a dependency
    # is incorrectly incorporated.
    if len(traces) > 10:
        traces = traces[0:10]

    # State so far:
    # got_providers: list (maybe empty) of metadata providers we directly have
    # trans_tmi: the list of the collection providers from our children.

    # Efficiently merge them.

    # We can do some tricks to avoid allocating a lot of memory
    # in big graphs. For the most part, metadata attachments are near the
    # leaves, and sparse higher up.
    # 1. If there is no direct metadata (got_providers is None) and there is
    #    no transitive metadata, return the null instance.  This is typical
    #    for home grown code that only depends on our own code.
    # 2. If got_providers is None, and there transitive info.
    #    If the length of the list is one, just pass up the first element.
    #    This is common through the whole middle of a build graph.
    # 3. If the above fail, construct a new one.

    if DEBUG_LEVEL > 0:
        print("%s: got: %d, trans: %d" % (target.label, len(got_providers), len(trans_tmi)))

    if not got_providers and not trans_tmi:
        return [null_provider_instance or provider_factory()]

    if not got_providers:
        """
        TODO: If there is only one, pass up the entire provider, not the extracted trans
        if len(trans_tmi) == 1 and trans_tmi[0]:
            # Often, there is only one thing we are passing up. There is no
            # reason to allocate another collection provider around that.
            return trans_tmi[0]
         """
        return [provider_factory(trans = depset(transitive = trans_tmi))]

    # Create a TWMI linking this target to the applicable metadata
    me = TargetWithMetadataInfo(
        target = target.label,
        metadata = depset(got_providers),
    )
    if not trans_tmi:
        return [provider_factory(
            trans = depset(direct = [me]),
        )]
    return [provider_factory(
        trans = depset(direct = [me], transitive = trans_tmi),
    )]

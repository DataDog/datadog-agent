"""
Show the Agent configuration enabled by a list of products, for a platform.

Products, their dependencies and the values they set are declared in the Agent schema (see 'product_dependencies',
'product_defaults' and 'product_platform_defaults'). The generated configuration only contains the settings whose
product value differs from the regular default: it's what a user would have to write to enable those products.
"""

import os

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.schema.lint import (
    PRODUCT_DEFAULT_KEYS,
    SCHEMA_DIR,
    _canonical_value,
    _product_value,
    get_product_definitions,
    resolve_product_closure,
    walk_nodes,
)
from tasks.schema.merge_schema import resolve_schema

# Platform keys by priority for each supported platform. This follows getPlatformDefault in
# pkg/config/setup/config.go (fargate, kubernetes, container, OS, other); containers are assumed to run Linux.
PLATFORMS = {
    "linux": ["linux", "other"],
    "windows": ["windows", "other"],
    "darwin": ["darwin", "other"],
    "aix": ["aix", "other"],
    "container": ["container", "linux", "other"],
    "kubernetes": ["kubernetes", "container", "linux", "other"],
    "fargate": ["fargate", "container", "linux", "other"],
}


def resolve_products(products, core_schema):
    """Return the sorted list of *products* with all their dependencies, or exit on invalid product names."""
    if not products:
        raise Exit("Error: at least one product is required.", code=1)
    sku_definitions, product_dependencies = get_product_definitions(core_schema)
    for product in products:
        if product in sku_definitions:
            raise Exit(f"Error: '{product}' is a SKU, not a product.", code=1)
        if product not in product_dependencies:
            raise Exit(f"Error: unknown product '{product}'.", code=1)
    return sorted(resolve_product_closure(products, product_dependencies))


def _regular_default(node, platform_keys):
    """Return the value a setting has when no product sets it, for the given platform priority."""
    if "default" in node:
        return node["default"]
    platform_default = node.get("platform_default")
    if isinstance(platform_default, dict):
        for key in platform_keys:
            if key in platform_default:
                return platform_default[key]
    return None


def _set_nested(config, dotted_key, value):
    *sections, name = dotted_key.split(".")
    for section in sections:
        config = config.setdefault(section, {})
    config[name] = value


def _schema_config(schema, products, platform_keys):
    """Return (nested configuration, conflicts) set by *products* in *schema*."""
    config = {}
    conflicts = []
    for node_path, node in walk_nodes(schema):
        if node.get("node_type") != "setting" or not any(key in node for key in PRODUCT_DEFAULT_KEYS):
            continue
        values = {}
        for product in products:
            found, value = _product_value(node, product, platform_keys)
            if found:
                values[product] = value
        if not values:
            continue
        if len({_canonical_value(v) for v in values.values()}) > 1:
            conflicts.append(f"{node_path}: " + ", ".join(f"{product}={value!r}" for product, value in values.items()))
            continue
        value = next(iter(values.values()))
        if _canonical_value(value) != _canonical_value(_regular_default(node, platform_keys)):
            _set_nested(config, node_path, value)
    return config, conflicts


def product_config(core_schema, system_probe_schema, platform, products):
    """
    Return the (datadog.yaml, system-probe.yaml) configurations enabled by *products* on *platform*, as nested
    dictionaries. Exits if the platform or a product is unknown, or if the products set conflicting values.
    """
    if platform not in PLATFORMS:
        raise Exit(f"Error: unknown platform '{platform}'. Use one of: {', '.join(PLATFORMS)}.", code=1)
    platform_keys = PLATFORMS[platform]
    resolved = resolve_products(products, core_schema)

    core_config, core_conflicts = _schema_config(core_schema, resolved, platform_keys)
    system_probe_config, system_probe_conflicts = _schema_config(system_probe_schema, resolved, platform_keys)
    conflicts = core_conflicts + system_probe_conflicts
    if conflicts:
        raise Exit(
            "Error: the products set conflicting values:\n  - " + "\n  - ".join(conflicts),
            code=1,
        )
    return core_config, system_probe_config


def render_config(core_config, system_probe_config, platform):
    """Render the configurations as YAML documents, one per configuration file."""
    documents = [
        f"# {name}\n" + yaml.safe_dump(config, default_flow_style=False, sort_keys=True)
        for name, config in (("datadog.yaml", core_config), ("system-probe.yaml", system_probe_config))
        if config
    ]
    if not documents:
        return f"# The products don't change the default configuration on '{platform}'.\n"
    return "---\n".join(documents)


def _table(title, rows):
    """Render (name, [names]) rows as two aligned columns."""
    width = max(len(title[0]), *(len(name) for name, _ in rows)) + 2
    lines = [f"{title[0]:<{width}}{title[1]}"]
    lines += [f"{name:<{width}}{', '.join(sorted(entries)) or '-'}" for name, entries in rows]
    return "\n".join(lines)


def list_products(core_schema):
    """Return the products declared in the core schema with their direct dependencies, then the SKUs if any."""
    sku_definitions, product_dependencies = get_product_definitions(core_schema)
    sections = [_table(("PRODUCT", "DEPENDS ON"), sorted(product_dependencies.items()))]
    if sku_definitions:
        sections.append(_table(("SKU", "PRODUCTS"), sorted(sku_definitions.items())))
    return "\n\n".join(sections) + "\n"


@task
def list_product(_ctx):
    """
    List the products declared in the Agent schema with the products they depend on, and the SKUs.

    Dependencies are resolved recursively when a product is enabled: see 'dda inv schema.show-product' for the resulting
    configuration.
    """
    print(list_products(resolve_schema(os.path.join(SCHEMA_DIR, "core_schema.yaml"))), end="")


@task(
    help={
        "platform": f"Platform to generate the configuration for: {', '.join(PLATFORMS)}.",
        "products": "Products to enable, separated by commas or spaces (e.g. 'apm,log_management').",
    },
    positional=["platform", "products"],
)
def show_product(_ctx, platform, products):
    """
    Show the configuration enabled by a list of products on a platform.

    Prints, as YAML, the datadog.yaml and system-probe.yaml settings that differ from the regular defaults once the
    products and their dependencies are enabled. Example: 'dda inv schema.show-product kubernetes apm,log_management'.
    """
    core_schema = resolve_schema(os.path.join(SCHEMA_DIR, "core_schema.yaml"))
    system_probe_schema = resolve_schema(os.path.join(SCHEMA_DIR, "system-probe_schema.yaml"))
    names = [name for name in products.replace(",", " ").split() if name]
    core_config, system_probe_config = product_config(core_schema, system_probe_schema, platform, names)
    print(render_config(core_config, system_probe_config, platform), end="")

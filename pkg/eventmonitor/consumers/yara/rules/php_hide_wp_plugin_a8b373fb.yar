rule php_hide_wp_plugin_a8b373fb {
    meta:
        author                    = "Taavi E"
        date                      = "2022-10-21"
        description               = "Detects WordPress plugins that are trying to hide themselves"
        version                   = "v0.1"
        tlp                       = "TLP:WHITE"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_author_email      = "taavi@zone.ee"
        yarahub_author_twitter    = "@TaaviEom"
        yarahub_license           = "CC BY-NC-SA 4.0"
        yarahub_uuid              = "7fce8c38-c9d5-44b9-96d9-7d6f22b41e07"
        yarahub_reference_md5     = "694647d78597f321318c9ea39a71a3b7"

    strings:
        $meta_1 = /add_filter\('all_plugins', '[a-z_A-Z]{1,30}'\);/
        $meta_2 = /function hide_plugin.{1,20}\(\$plug.{1,5}\) {/
        $meta_3 = /unset\(\$plugins\['[a-z_A-Z]{2,20}\/[a-z_A-Z]{2,20}.php'\]\);/
        $meta_4 = /return \$[a-z_A-Z]{1,20};/
    condition:
        $meta_1 or $meta_2 or ($meta_3 and $meta_4)
}

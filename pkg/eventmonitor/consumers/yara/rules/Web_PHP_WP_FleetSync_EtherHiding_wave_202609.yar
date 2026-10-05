rule Web_PHP_WP_FleetSync_EtherHiding_wave_202609
{
    meta:
        author = "Serhii Kocherhan"
        yarahub_twitter = "@skocherhan"
        date = "2026-09-23"
        description = "Current-wave strings for WP FleetSync implant. Expect rotation."
        yarahub_uuid = "e8614680-729e-49d9-acb1-d03b0e985a2f"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        $a = "wp-helper-a220ab" ascii
        $b = "likingdropout.site" ascii
        $c = "X-HSS-Auth" ascii
        $d = "function _ea_sync(" ascii
        $e = "0x38bd65e2" ascii

    condition:
        filesize < 200KB and 3 of them
}
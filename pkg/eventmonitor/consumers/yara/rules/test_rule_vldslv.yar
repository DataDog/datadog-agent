rule test_rule_vldslv
{
    meta:
        date = "2026-04-10"
        yarahub_uuid = "943e8da1-0f2a-45fa-91a9-c20736909404"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d41d8cd98f00b204e9800998ecf8427e"

    strings:
        $a = "malware"

    condition:
        $a
}
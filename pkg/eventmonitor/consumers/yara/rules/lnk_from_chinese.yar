

rule lnk_from_chinese : odd {
    meta:
        category = "apt"
        description = "what the rule does"
        author = "malcat"
        reliability = 50
        date = "2022-07-04"
        yarahub_uuid = "17a4f2d6-0792-45de-8b90-749bec1bcc18"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e3f89049dc5f0065ee4d780f8aef9c04"
    strings:
        $magic = { 4C0000000114020000000000C000000000000046 }
        $serial = {90962EBA}
    condition:
        $magic at 0 and $serial
}

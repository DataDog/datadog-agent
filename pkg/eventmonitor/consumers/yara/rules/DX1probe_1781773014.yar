rule DX1probe_1781773014
{
    meta:
        description = "DX1desc\"><img src=x onerror=window.__dx1=1>"
        author = "DX1auth\"><svg onload=1>"
        reference = "DX1ref'\"><b>x</b>"
        date = "2026-06-18"
        yarahub_reference_md5 = "8e874d15b874771345e8d02b13350a91"
        yarahub_uuid = "815a215c-c097-46e8-af13-9608ad1ecb63"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
    strings:
        $a = "DX1uniquestringmarkerZZ"
    condition:
        $a
}

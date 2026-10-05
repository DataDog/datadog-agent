rule easyforme_infostealer : infostealer easyforme java malicious
{
    meta:
        date = "2025-05-29"
        yarahub_uuid = "d7e2a1dd-5f7f-4b9f-a16a-d19ad4d5c25a"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "8f10a948513313d932b37f8991322aa9"

    strings:
        $zip_header = {50 4B 03 04}
        $classname = "xvbspjygolenxfxo/tpxtvsxcreorluhz.class"

    condition:
        for any i in (1..#zip_header):
        (
            $classname in (@zip_header[i] + 30 .. @zip_header[i] + 30 + uint16(@zip_header[i] + 26))
        )
}
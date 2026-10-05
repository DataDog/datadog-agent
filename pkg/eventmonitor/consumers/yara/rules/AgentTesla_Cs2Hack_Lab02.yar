rule AgentTesla_Cs2Hack_Lab02 {
    meta:
        author = "Francisco"
        description = "Detecta variante de AgentTesla disfrazada como 'Cs2Hack.exe'"
        date = "2026-06-21"
        hash = "49457f4873d15debba87933ff93d9709c820d2e3ca35658af3cff7c26b62dda4"
        yarahub_uuid = "4f2a1e8d-6c3b-4a91-8d4f-2c9b7e1a5f63"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "ec83b6003cb6fb9a31a342938e54446f"
    strings:
        $c2 = "litter.catbox.moe"
        $fake_company = "BrightView North Apps"
        $fake_desc = "screenshot finder"
    condition:
        uint16(0) == 0x5A4D and 2 of ($c2, $fake_company, $fake_desc)
}
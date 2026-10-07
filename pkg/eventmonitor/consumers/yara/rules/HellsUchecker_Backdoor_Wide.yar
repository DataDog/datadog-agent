rule HellsUchecker_Backdoor_Wide
{
    meta:
        id = "V1kgj1znQLmaFSSdZ2PcEA"
        fingerprint = "f653285ece03301efc6e4fba4761387cb6af3b52acde1b288f557d8fac4070a8"
        version = "1.0"
        date = "2026-03-11"
        modified = "2026-03-11"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "HellsUchecker backdoor - broader detection without PDB path. Targets the unique combination of JSON-RPC template, custom User-Agent, and URL encoding pattern."
        category = "MALWARE"
        malware = "HELLSUCHECKER"
        mitre_att = "T1071.001"
        reference = "https://derp.ca/blog/hellsuchecker-clickfix-etherhiding"
        triage_score = 10
        triage_description = "Probable HellsUchecker backdoor detected. Custom JSON-RPC C2 implant."
        yarahub_uuid = "de95a1ae-826b-40cb-9ffd-1c35a944b96c"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "465d5847030acbbe1b6aaad39088d2f1"

    strings:
        $jsonrpc_template = "{\"id\":,\"arguments\":[]}" ascii
        $ua_checkin = "myApp v1.0" ascii
        $ct_json = "Content-Type: application/json\r\n" ascii
        $fingerprint_prefix = "USERNAME=" ascii
        $day_lut = "SunMonTueWedThuFriSat" ascii
        $month_lut = "JanFebMarAprMayJunJulAugSepOctNovDec" ascii

    condition:
        uint16(0) == 0x5A4D
        and filesize < 100KB
        and $jsonrpc_template
        and $ua_checkin
        and $ct_json
        and ($day_lut or $month_lut)
        and $fingerprint_prefix
}

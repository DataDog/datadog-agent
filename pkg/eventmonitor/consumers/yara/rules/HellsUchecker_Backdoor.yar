rule HellsUchecker_Backdoor
{
    meta:
        id = "XaKgn7Morr_pIPI_rOh_05"
        fingerprint = "549f157a1defdbefde0923a8cd73ed9dca028297e2f26074c0a0473915307d12"
        version = "1.0"
        date = "2026-03-11"
        modified = "2026-03-11"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "HellsUchecker native x64 backdoor - JSON-RPC HTTPS C2 with custom URL encoding. Memory-resident only (never written to disk). Scan process memory or dumps. PDB: uchecker.pdb."
        category = "MALWARE"
        malware = "HELLSUCHECKER"
        mitre_att = "T1071.001"
        reference = "https://derp.ca/blog/hellsuchecker-clickfix-etherhiding"
        triage_score = 10
        triage_description = "HellsUchecker backdoor detected in memory. Custom native x64 implant with JSON-RPC C2."
        yarahub_uuid = "2a2ffbb9-e164-48c4-bae0-958e07988711"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "465d5847030acbbe1b6aaad39088d2f1"

    strings:
        $pdb = "uchecker.pdb" ascii
        $jsonrpc_template = "{\"id\":,\"arguments\":[]}" ascii
        $ua_checkin = "myApp v1.0" ascii
        $ct_json = "Content-Type: application/json\r\n" ascii
        $fingerprint_prefix = "USERNAME=" ascii

    condition:
        uint16(0) == 0x5A4D
        and filesize < 100KB
        and $pdb
        and $jsonrpc_template
        and 2 of ($ua_checkin, $ct_json, $fingerprint_prefix)
}

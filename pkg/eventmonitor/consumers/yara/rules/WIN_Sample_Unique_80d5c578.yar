rule WIN_Sample_Unique_80d5c578
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (pe, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "80d5c57843523754b5fce827c12d3c922810f2175a8d6189ef944e294c76d49c"
        yarahub_uuid = "2278a6e2-72e9-4930-aa20-5a20bde5e3e8"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "pe"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9926feb76c3bf72ea7131a3588c42b80"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "Caching options..." ascii
        $s1 = "Scanning credentials..." ascii
        $s2 = "Processing components..." ascii
        $s3 = "Reloading settings..." ascii
        $s4 = "Preparing components..." ascii
        $s5 = "Processing fonts..." ascii
        $s6 = "Refreshing options..." ascii
        $s7 = "Processing credentials..." ascii
        $s8 = "Rendering schemas..." ascii
        $s9 = "Compressing options..." ascii
        $s10 = "Encoding extensions..." ascii
        $s11 = "Computing shortcuts..." ascii
        $s12 = "Updating windows..." ascii
        $s13 = "Updating filters..." ascii
        $s14 = "Connecting filters..." ascii
        $s15 = "Optimizing resources..." ascii
        $s16 = "Writing layouts..." ascii
        $s17 = "Preparing credentials..." ascii
        $s18 = "Reloading documents..." ascii
        $s19 = "Sorting documents..." ascii
    condition:
        uint16(0) == 0x5A4D and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}

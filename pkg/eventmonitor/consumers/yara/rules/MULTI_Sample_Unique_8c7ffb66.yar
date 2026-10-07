rule MULTI_Sample_Unique_8c7ffb66
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (inconnu, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "8c7ffb66a2aebb8a67ddd885f9b3279cdfc8909e8a4350d2bcabfb1ea4c96709"
        yarahub_uuid = "084838bf-8492-4598-b867-94c230654741"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "inconnu"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "0ffa43cedea6cb8e4545c1f2eb102fc0"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "\"file\": \"foure:xuong.ttf\"," ascii
        $s1 = "\"shift\": [0, -1]," ascii
        $s2 = "\"id\": \"minecraft:include/default\"" ascii
        $s3 = "\"providers\": [" ascii
        $s4 = "\"oversample\": 8" ascii
        $s5 = "\"type\": \"ttf\"," ascii
        $s6 = "\"size\": 11," ascii
        $s7 = "\"type\": \"reference\"," ascii
    condition:
        true and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}

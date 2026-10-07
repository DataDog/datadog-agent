rule JAVA_Sample_Unique_1dbb7618
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "1dbb7618594f15768674410532b8dd8b7e5923d9e5ec9cade64294ef9ee66956"
        yarahub_uuid = "efe5b641-6954-4270-a46a-5b5e5276758e"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "5a526d418447748e7c47f99ad1eced5e"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/esp/BlockNotifier$Mode" ascii
        $s1 = "com/nnpg/glazed/modules/esp/BlockNotifier" ascii
        $s2 = "1[Lcom/nnpg/glazed/modules/esp/BlockNotifier$Mode;" ascii
        $s3 = "3()[Lcom/nnpg/glazed/modules/esp/BlockNotifier$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3)
}

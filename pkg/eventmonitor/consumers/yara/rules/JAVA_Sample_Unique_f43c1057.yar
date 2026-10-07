rule JAVA_Sample_Unique_f43c1057
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "f43c1057d60b052b9a16dfff4f414946bf35f66088e12b819745cb81f8381d4e"
        yarahub_uuid = "98c8672b-62ad-4802-ad59-68aa6dbaa57a"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "92e0b84b14a9ecfdf5ba080d53fe5e78"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/main/RainNoti$Mode" ascii
        $s1 = "com/nnpg/glazed/modules/main/RainNoti" ascii
        $s2 = ")[Lcom/nnpg/glazed/modules/main/RainNoti$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them
}

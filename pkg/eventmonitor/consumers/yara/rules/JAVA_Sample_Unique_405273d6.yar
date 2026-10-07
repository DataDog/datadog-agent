rule JAVA_Sample_Unique_405273d6
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "405273d65b8dc9b3075287dc09dcc9d78b16c90275f2d2933c5d1f24fa519c17"
        yarahub_uuid = "696c20ef-4f12-435b-93b4-0dc10de770a9"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9568c5811073c50f94be88c1488d4708"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "com/nnpg/glazed/modules/esp/WanderingESP$Mode" ascii
        $s1 = "com/nnpg/glazed/modules/esp/WanderingESP" ascii
        $s2 = "0[Lcom/nnpg/glazed/modules/esp/WanderingESP$Mode;" ascii
        $s3 = "2()[Lcom/nnpg/glazed/modules/esp/WanderingESP$Mode;" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3)
}

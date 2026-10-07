rule JAVA_Sample_Unique_753fc60d
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (class, etat binaire)"
        date = "2026-09-25"
        reference_sha256 = "753fc60df0da2083211545487ff00fb87daf169f8c545f2e229746eda08a2133"
        yarahub_uuid = "5f5b3b93-13e0-496b-b8e1-d239ad43f448"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "class"
        etat = "binaire"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.44"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "69c4197869c459f1b0e6eea1e39f34f2"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = ")Lcom/dclient/module/modules/client/d;" ascii
        $s1 = "com/dclient/client/gui/ThemeUtil" ascii
        $s2 = "com/dclient/module/ModuleManager" ascii
        $s3 = "accentDark" ascii
        $s4 = "cachedAccent" ascii
        $s5 = "cachedTheme" ascii
        $s6 = "getByName" ascii
        $s7 = "getTheme" ascii
        $s8 = "lastCacheMs" ascii
    condition:
        uint32(0) == 0xBEBAFECA and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}

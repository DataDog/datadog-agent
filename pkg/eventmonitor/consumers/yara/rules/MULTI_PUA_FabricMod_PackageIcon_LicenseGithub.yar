rule MULTI_PUA_FabricMod_PackageIcon_LicenseGithub {
    meta:
        description = "Mod Minecraft Fabric (structure package + LICENSE_github) soumis en masse - PUA/mods, pattern de soumission"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "misses_archive / grappe 9 archives"
        confidence  = "medium"
        note        = "Meme famille que ToggleSprint : mods soumis massivement. Verifier la charge utile en sandbox."
        yarahub_uuid            = "0562653c-5984-4bfd-9b52-f97f0bf1a648"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "fe327e91fb8557e6c35cc6fa8958bb15"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $zip = { 50 4B 03 04 }
        $a   = "assets/package/icon.png" ascii wide
        $b   = "LICENSE_github" ascii wide
    condition:
        $zip and $a and $b
}

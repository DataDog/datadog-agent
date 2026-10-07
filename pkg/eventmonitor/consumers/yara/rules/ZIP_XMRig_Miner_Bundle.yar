rule ZIP_XMRig_Miner_Bundle {
    meta:
        description = "Bundle XMRig (mineur non consenti) - archive avec WinRing0 driver"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / bundle xmrig"
        confidence  = "medium"
        yarahub_uuid            = "b7c8eeee-53c6-4ba5-b8ee-1465d555a811"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "f61e7fc8b9e8243577bdc0a601fb22f3"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $zip = { 50 4B 03 04 }
        $x1  = "xmrig-v2/WinRing0x64.sys"
        $x2  = "xmrig-v2/start.cmd"
    condition:
        $zip and 1 of ($x1, $x2)
}

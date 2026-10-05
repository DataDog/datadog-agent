rule lsi_remcos2
{
    meta:
        date = "2024-12-03" 
        author = "anonym"
        description = "Remcos_V5 Payload"
        yarahub_reference_md5 = "f9c6ffd9d3156c7701ddcceb42181ee3"
        yarahub_license = "CC0 1.0"
        yarahub_uuid = "41447723-de68-43e7-bfcd-aaa4fb55f2eb"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        malpedia_family = "win.remcos"
    strings:
        $name  = "Remcos" nocase
        $time   = "%02i:%02i:%02i:%03i"
        $crypto1 = {81 E1 FF 00 00 80 79 ?? 4? 81 C9 00 FF FF FF 4? 8A ?4 8?}
        $crypto2 = {0F B6 [1-7] 8B 45 08 [0-2] 8D 34 07 8B 01 03 C2 8B CB 99 F7 F9 8A 84 95 ?? ?? FF FF 30 06 47 3B 7D 0C 72}
		$version = {00 35 2E (?? | ?? 2E ??) 20 [2-5] 00}
    condition:
        uint16(0) == 0x5A4D and ($name) and ($time) and ($version) and any of ($crypto*)
}

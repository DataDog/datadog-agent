rule botnet_Yakuza {
    meta:
        author = "NDA0E"
        date = "2024-07-22"
	description = "Yakuza botnet"
        yarahub_uuid = "c0ed7b7d-f8f5-4301-812d-aaca80577c97"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "6dde652b28f73f978e834412b835a740"
    strings:
	$yakuza = "Self Rep Fucking NeTiS and Thisity 0n Ur FuCkInG FoReHeAd We BiG L33T HaxErS" ascii
	$YakuzaBotnet = "YakuzaBotnet" ascii
    condition: 
	uint16(0) == 0x457f and any of them
}
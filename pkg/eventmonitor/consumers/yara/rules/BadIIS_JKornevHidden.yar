rule BadIIS_JKornevHidden {
	meta:
		author = "Still"
		component_name = "JKornevHidden"
		date = "2025-09-20"
		description = "attempts to match the strings found in BadIIS variant of the JKornevHidden rootkit"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "2965ddbcd11a08a3ca159af187ef754c"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "2ad4b588-51c4-4c64-ac24-de6f75047619"
	strings:
		$str_1 = "_Zhuangtai" wide
		$str_2 = "_YinshenMode" wide
		$str_3 = "_WinkbjRegValues" wide
		$str_4 = "_FangxingImages" wide
		$str_5 = "_BaohuImages" wide
		$str_6 = "[HahaDbg]" wide ascii
		$str_7 = "\\\\DosDevices\\\\WinkbjDamen" wide 
	condition:
		3 of them
}
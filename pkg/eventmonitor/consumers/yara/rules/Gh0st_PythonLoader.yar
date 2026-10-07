rule Gh0st_PythonLoader
{
	meta:
		author = "Still"
		component_name = "Gh0st"
		date = "2025-04-12"
		malpedia_family = "win.ghost_rat"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "2dc68441b200ee3014a40c95e2dfc6e1"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "64863e2c-fd88-4077-8a8c-fd196d84a6ea"
		description = "Matches an unknown Gh0st variant Python loader"
	strings:
		$str_1 = "def ecute(lcode):" ascii fullword
		$str_2 = "No to cute." ascii fullword
		$str_3 = "Always cute when imported" ascii fullword
		$str_4 = "code_size, 0x40, ctypes.byref(old_protect)" ascii fullword
		$str_5 = "code_func = codeFunction(" ascii fullword
		$str_6 = "None, code_size, 0x3000, 0x04)" ascii fullword
	condition:
		3 of ($str_*)
}

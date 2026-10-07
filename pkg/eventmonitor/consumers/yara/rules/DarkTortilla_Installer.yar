rule DarkTortilla_Installer
{
	meta:
		author = "Still"
		component_name = "N/A"
		date = "2025-01-11"
		malpedia_family = "win.darktortilla"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "CE23E784C492814093F9056ABD00080F"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "0bff8b1c-2fbc-451b-b9cd-999c5365f163"
		description = "Matches DarkTortilla installer strings/bytecode"
	strings:
		$str_1 = "%Compress%" ascii fullword
		$str_2 = "%InjectionPersist%" ascii fullword
		$str_3 = "icompleted" ascii fullword
		$str_4 = "icomplete.exe" ascii fullword
	condition:
		3 of ($str_*)
}

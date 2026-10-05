rule Suspicious_PowerShellObjectCreation
{
	meta:
		date = "2025-02-13"
		yarahub_author_twitter = "@AzakaSekai_"
		yarahub_license = "CC BY-NC 4.0"
		yarahub_reference_md5 = "7ee13c839f3af9ca9a4e8b692f7018fa"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_uuid = "c88d3adb-5b6e-4b9a-b7fc-e15a409a55ad"
	strings:
		$base_1 = /\$ExecutionContext ?\| ?(Get-Member|gm)/ ascii nocase
		$optional_1 = "GetCommand" ascii nocase fullword
		$optional_2 = "Cmdlet" ascii nocase fullword
		$optional_3 = "PsObject" ascii nocase fullword
		$optional_4 = ")[6].Name)" ascii nocase fullword
	condition:
		$base_1 and
		2 of ($optional_*)
}

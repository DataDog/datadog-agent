rule RANSOM_Magniber_LNK_Jan23
{
	meta:
		author = "SECUINFRA Falcon Team"
		description = "Detects Magniber Ransomware LNK files from fake Windows Update delivery method"
		reference = "https://twitter.com/SI_FalconTeam/status/1613540054382559234"
		date = "2023-01-13"
		tlp = "CLEAR"
		hash = "16ecec4efa2174dec11f6a295779f905c8f593ab5cc96ae0f5249dc50469841c"
		yarahub_uuid = "ceee9545-c008-41d8-bc2f-513e78209d21"
        yarahub_reference_md5 = "fedb6673626b89a9ee414a5eb642a9d9"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = "@SI_FalconTeam"

	strings:
		$netbiosName = "victim1" ascii fullword
		$macAddress = {00 0C 29 07 E1 6D}
	
	condition:
		uint32be(0x0) == 0x4C000000 
		and all of them
}
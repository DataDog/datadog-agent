rule RANSOM_Magniber_ISO_Jan23
{
	meta:
		author = "SECUINFRA Falcon Team"
		description = "Detects Magniber Ransomware ISO files from fake Windows Update delivery method"
		reference = "https://twitter.com/SI_FalconTeam/status/1613540054382559234"
		date = "2023-01-13"
		tlp = "CLEAR"
		hash = "4dcbcc070e7e3d0696c777b63e185406e3042de835b734fe7bb33cc12e539bf6"
		yarahub_uuid = "19686301-e651-4bfe-b295-712a90f3156c"
        yarahub_reference_md5 = "fedb6673626b89a9ee414a5eb642a9d9"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = "@SI_FalconTeam"

	strings:
		$magic = {43 44 30 30 31} // CD001 ISO Magic
		$tool = {55 4C 54 52 41 49 53 4F 00 39 2E 37 2E 36 2E 33 38 32 39} // "ULTRAISO.9.7.6.3829"

		$msiMagic = {D0 CF 11 E0 A1 B1 1A E1}
		$dosString = "!This program cannot be run in DOS mode" ascii // To "exclude" Office files which also use $msiMagic
		$lnkMagic = {4C 00 00 00}

	condition:
		filesize > 200KB 
		and filesize < 800KB 
		and all of them
}
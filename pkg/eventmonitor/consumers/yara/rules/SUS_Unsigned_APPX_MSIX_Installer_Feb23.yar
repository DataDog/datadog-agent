rule SUS_Unsigned_APPX_MSIX_Installer_Feb23
{
	meta:
		author = "SECUINFRA Falcon Team (@SI_FalconTeam)"
		description = "Detects suspicious, unsigned Microsoft Windows APPX/MSIX Installer Packages"
		reference = "https://twitter.com/SI_FalconTeam/status/1620500572481945600"
		date = "2023-02-01"
		tlp = "CLEAR"
		yarahub_reference_md5 = "69660f5abb08fc430cf756a44d19e039"
		yarahub_uuid = "3eaac733-4ab9-40e1-93fe-3dbed6d458e8"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_author_twitter = "@SI_FalconTeam"

	strings:
		$s_manifest = "AppxManifest.xml"
		$s_block = "AppxBlockMap.xml"
		$s_peExt = ".exe"

		// we are not looking for signed packages
		$sig = "AppxSignature.p7x"

	condition:
		uint16be(0x0) == 0x504B
		and 2 of ($s*)
		and not $sig
}

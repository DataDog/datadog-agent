rule SUS_Unsigned_APPX_MSIX_Manifest_Feb23
{
	meta:
		author = "SECUINFRA Falcon Team (@SI_FalconTeam)"
		description = "Detects suspicious Microsoft Windows APPX/MSIX Installer Manifests"
		reference = "https://twitter.com/SI_FalconTeam/status/1620500572481945600"
		date = "2023-02-01"
		tlp = "CLEAR"
		yarahub_reference_md5 = "69660f5abb08fc430cf756a44d19e039"
		yarahub_uuid = "06b5fba4-6b6d-41f8-9910-cce86eabbde4"
		yarahub_license = "CC BY 4.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_author_twitter = "@SI_FalconTeam"

	strings:
		$xlmns = "http://schemas.microsoft.com/appx/manifest/"
		
		// as documented here: https://learn.microsoft.com/en-us/windows/msix/package/unsigned-package
		$identity = "OID.2.25.311729368913984317654407730594956997722=1"
		
		$s_entrypoint = "EntryPoint=\"Windows.FullTrustApplication\""
		$s_capability = "runFullTrust"
		$s_peExt = ".exe"

	condition:
		uint32be(0x0) == 0x3C3F786D
		and $xlmns
		and $identity
		and 2 of ($s*)
}
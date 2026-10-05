rule caspratique_lumma
{
	meta:
		date = "2024-12-10"
		yarahub_uuid = "78403785-afdc-4e20-aef6-a635f7bdd129"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "37363b24a0f1a339cf2e9a3dba0e12e2"

	strings:
		$c1 = "impend-differ.biz" wide ascii
		$c2 = "print-vexer.biz" wide ascii
		$c3 = "dare-curbys.biz" wide ascii
		$c4 = "covery-mover.biz" wide ascii
		$c5 = "formy-spill.biz" wide ascii
		$c6 = "dwell-exclaim.biz" wide ascii
		$c7 = "zinc-sneark.biz" wide ascii
		$c8 = "se-blurry.biz" wide ascii
		$c9 = "atten-supporse.biz" wide ascii
		$i1 = "185.215.113.206" ascii
		$i2 = "185.215.113.16" ascii
		$i3 = "185.215.113.43" ascii
		$i4 = "31.41.244.11" ascii
		$a1 = "UF1YUJJQWDEUA036IMVNJ.exe" fullword ascii
		$mz = {4D 5A}

	condition:
		(1 of ($c*) or 1 of ($i*) or 1 of ($a*) and $mz)
}
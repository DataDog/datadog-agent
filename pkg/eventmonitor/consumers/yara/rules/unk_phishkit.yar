rule unk_phishkit {
	meta:
		author = "James E.C, Proofpoint"
		description = "Unknown phishkit"
		date = "2022-07-06"
		yarahub_uuid = "c6d0afdc-2d5e-4674-bca0-5e6738c22bca"
		yarahub_license = "CC0 1.0"
		yarahub_rule_matching_tlp = "TLP:WHITE"
		yarahub_rule_sharing_tlp = "TLP:WHITE"
		yarahub_reference_md5 = "7639fdbeac0f75cbcbd9b623a8a6b0d6"
	strings:
		$hp1 = "function validateMyForm()" ascii
		$hp2 = ".getElementById(\"honeypot\").value" ascii

		$kit1 = /<form action=\"[A-Za-z0-9]{2,8}\.php\"/
		$kit2 = "onSubmit=\"return validateMyForm();" ascii
		$kit3 = "id='_form_" ascii
		$kit4 = "enctype='multipart/form-data'" ascii
	condition:
		filesize < 50KB and all of them
}
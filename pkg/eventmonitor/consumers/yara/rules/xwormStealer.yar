rule xwormStealer {

  meta:
      author = "Jeffrey Farnan"
      description = " Infostealer / backdoor"
      date = "2024-04-11"
      yarahub_author_twitter = "@jeffrey_farnan"
      yarahub_author_email = "jfarnan@opentext.com"
      yarahub_reference_link = "https://twitter.com/jeffrey_farnan"
      yarahub_reference_md5 = "ff9e45d7326698f34526793bf1244811"
      yarahub_uuid = "c535499f-a603-4178-a069-8c70ccc3fbc7"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "XWorm"

strings:


	$s1 = "OPHt.exe"
	$s2 = "cserver=40.76.205.114"
	$s3 = "$14fd9586-59f9-419a-91fa-4fec2c6f81f6"
	

condition:
	2 of ($s*)


}

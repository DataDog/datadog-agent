rule Guloader_VBScript {

  meta:
      author = "Ankit Anubhav - ankitanubhav.info"
      description = "Detects GuLoader/CloudEye VBScripts"
      date = "2022-07-14"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav"
      yarahub_reference_md5 = "00e59c5ea76face15c42450c71676e03"
      yarahub_uuid = "7d7e2b7c-5536-4688-b202-e79c401e7195"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.CloudEye"

strings:


	$x = { 20 26 20 22 }
	$y = { 54 69 6d 65 56 61 6c 75 65 28 22 ( 31 3a 31 3a 31 | 32 3a 32 3a 32 | 33 3a 33 3a 33 | 34 3a 34 3a 34 | 35 3a 35 3a 35 | 36 3a 36 3a 36 | 37 3a 37 3a 37 | 38 3a 38 3a 38 | 39 3a 39 3a 39 | 31 30 3a 31 30 3a 31 30 | 31 31 3a 31 31 3a 31 31 | 31 32 3a 31 32 3a 31 32 | 31 33 3a 31 33 3a 31 33 | 31 34 3a 31 34 3a 31 34 | 31 35 3a 31 35 3a 31 35 | 31 36 3a 31 36 3a 31 36 | 31 37 3a 31 37 3a 31 37 | 31 38 3a 31 38 3a 31 38 | 31 39 3a 31 39 3a 31 39 | 32 30 3a 32 30 3a 32 30 | 32 31 3a 32 31 3a 32 31 | 32 32 3a 32 32 3a 32 32 | 32 33 3a 32 33 3a 32 33 ) 22 29 }
	//$z = { 44 69 6d } new variants have started using loose binding so commenting out this line !!
condition:
	#x > 20 and $y and filesize < 1999999


}

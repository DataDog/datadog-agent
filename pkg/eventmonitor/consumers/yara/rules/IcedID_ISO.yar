rule IcedID_ISO {

  meta:
      author = "Ankit Anubhav - ankitanubhav.info"
      description = "Detects IcedID ISO archives"
      date = "2022-08-18"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav"
      yarahub_reference_md5 = "d5f065d3ac9dc75041af218718f4950e"
      yarahub_uuid = "53d04c1d-fd1a-4928-ae92-adfcc62dc029"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.icedid"

strings:


	$iso = "This disc contains"
	$exe = "This program cannot be run"
	$txrun = {74 78 74 2c 22}

condition:
	$iso and $exe and $txrun and filesize < 999999


}

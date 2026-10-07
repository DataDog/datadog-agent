rule LATAMHotel_Obfuscated_BAT {

  meta:
      author = "Ankit Anubhav - ankitanubhav.info"
      description = "Detects a campaign targeted towards LatinAmerican Hotels,generally leading to AsyncRAT"
      date = "2022-07-23"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://threatresearch.ext.hp.com/stealthy-opendocument-malware-targets-latin-american-hotels/"
      yarahub_reference_md5 = "00e59c5ea76face15c42450c71676e03"
      yarahub_uuid = "a31088bd-4baf-4f99-a89a-08f03389110b"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.asyncrat"

strings:


	$x = "1%%"
	$y = /~[0-9]{1,2}/
	$z = /=[A-Za-z0-9]{62}/

condition:
	#x > 90 and #y > 90 and $z  and filesize <30000


}

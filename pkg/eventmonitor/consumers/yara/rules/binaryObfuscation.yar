rule binaryObfuscation
{
  meta:
    author = 			"Sean Dalnodar"
    date = 			"2022-05-27"
    yarahub_uuid = 		"3f562951-b59f-4b27-806e-823e99910cac"
    yarahub_license =		"CC0 1.0"
    yarahub_rule_matching_tlp =	"TLP:WHITE"
    yarahub_rule_sharing_tlp = 	"TLP:WHITE"
    yarahub_reference_md5 =	"9c817fe677e2505306455d42d081252c"

  strings:
    $re0 = /=\([0-1,]{512}/

  condition:
    all of them
}
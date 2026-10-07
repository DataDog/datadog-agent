rule DynoWiper
{
meta:
author = "CERT Polska"
yarahub_reference_md5 = "a727362416834fa63672b87820ff7f27"
yarahub_uuid = "6e8a1b4a-5a3e-47ef-9785-95852a9ea794"
yarahub_license = "CC0 1.0"
yarahub_rule_matching_tlp = "TLP:WHITE"
yarahub_rule_sharing_tlp = "TLP:WHITE"
date = "2025-12-31"
hash = "4ec3c90846af6b79ee1a5188eefa3fd21f6d4cf6"
hash = "86596a5c5b05a8bfbd14876de7404702f7d0d61b"
hash = "69ede7e341fd26fa0577692b601d80cb44778d93"
hash = "0e7dba87909836896f8072d213fa2da9afae3633"
strings:
$a1 = "$recycle.bin" wide
$a2 = "program files(x86)" wide
$a3 = "perflogs" wide
$a4 = "windows\x00" wide
$b1 = "Error opening file: " wide
condition:
uint16(0) == 0x5A4D
and
filesize < 500KB
and
4 of them
}

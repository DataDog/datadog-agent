rule QakBot_OneNote_Loader {

  meta:
      author = "Ankit Anubhav - ankitanubhav.info"
      description = "Detects a OneNote malicious loader mostly used by QBot (TA570/TA577)"
      date = "2023-02-04"
      yarahub_author_twitter = "@ankit_anubhav"
      yarahub_author_email = "ankit.yara@inbox.ru"
      yarahub_reference_link = "https://twitter.com/ankit_anubhav"
      yarahub_reference_md5 = "b6c8d82a4ec67398c756fc1f36e32511"
      yarahub_uuid = "cbbe7ec6-1658-4f4b-b229-8ade27bff9f4"
      yarahub_license = "CC0 1.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
      malpedia_family = "win.qakbot"

strings:

  $x = { E4 52 5C 7B 8C D8 A7 4D AE B1 53 78 D0 29 96 D3 } // OneNote header

// Variant 1
// Looking for evidence of onenote containing vbs/js/ and code to write data in registry and execute it.
// Some of these might be obfuscated so looking for a 3/5 match.
  $a = "javascript" nocase
  $b = "vbscript" nocase
  $c = "regread" nocase
  $d = "regwrite" nocase
  $e = "RegDelete" nocase

// Variant 2
// Instead of hta abuses batch and powershell to download and run the DLL

  $f = ".cmd&&start /min" nocase //edit 07.02.22 for batch file vector
  $f2 = "&&cmd /c start /min" nocase // edit 14.02.22 run command and then exit
  $g = "powershell" nocase

// Variant 3
// Involves powershell as well but obfuscation is different.
// The string powershell can not be found because it is partially hidden by environment variables.

  $tok1 = "rundll32 C:\\ProgramData\\" nocase // tok1 botnet ID

// Some cases they are obfuscating a lot by breaking all in set

$h = "set" // Look for several of these
$i = "start /min"



condition:
	$x and ((3 of ($a,$b,$c,$d,$e)) or (($f or $f2) and $g) or $tok1 or (#h > 15 and $i))


}

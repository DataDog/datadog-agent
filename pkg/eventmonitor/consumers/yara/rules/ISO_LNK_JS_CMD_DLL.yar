rule ISO_LNK_JS_CMD_DLL {
   meta:
      description = "Detects iso > lnk > js > cmd > dll execution chain"
      author = "_jc"
      date = "2022-09-29"
      yarahub_reference_md5 = "b93bd94b8f568deac0143bf93f7d8bd8"
      yarahub_uuid = "3e54dac2-910d-4dda-a3b4-2fa052556be7"
      yarahub_license = "CC BY 4.0"
      yarahub_rule_matching_tlp = "TLP:WHITE"
      yarahub_rule_sharing_tlp = "TLP:WHITE"
   strings:
      $lnk_header = { 4C 00 }
	  $minimized_inactive = {07}
	  $js_ext = ".js" nocase

	  $echo_off = { 40 65 63 68 6F [32-64] 33 32} // "@echo..32" to catch .cmd + regsvr32 stitching

	  $js_var = {76 61 72 [1-32] 3D [1-16] 3B} // catches javascript-style variable declaration

	  $mz_dos_mode = {4D 5A [100-110] 44 4F 53 20 6D 6F 64 65} // catches MZ..DOS Mode

   condition:
      // spot minimized_inactive flag; invocation of .js file by lnk
	  $echo_off and $js_var and $mz_dos_mode and
      for any i in (1..#lnk_header):
	  (($minimized_inactive in (@lnk_header[i]+60..@lnk_header[i]+61)) and ($js_ext in (@lnk_header[i]+255..@lnk_header[i]+304)))
}
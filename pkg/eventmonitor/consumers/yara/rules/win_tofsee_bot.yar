rule win_tofsee_bot
{
  meta:
    author       = "akrasuski1"
    published_at = "https://gist.github.com/akrasuski1/756ae39f96d2714087e6d7f252a95b19"
    revision_by  = "andretavare5"
    description  = "Tofsee malware"
    org          = "BitSight"
    date         = "2023-03-24"
	yarahub_author_twitter =    "@andretavare5"
    yarahub_reference_link =    "https://www.bitsight.com/blog/tofsee-botnet-proxying-and-mining"
    yarahub_malpedia_family =   "win.tofsee"
    yarahub_uuid =              "bc8f6b49-01a2-467a-a619-960fc2cb5f7f"
    yarahub_license =           "CC BY-NC-SA 4.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp =  "TLP:WHITE"
    yarahub_reference_md5 =     "92e466525e810b79ae23eac344a52027"

  strings:
    $decryptStr  = {32 55 14 88 10 8A D1 02 55 18 F6 D9 00 55 14}
    $xorGreet    = {C1 EB 03 C0 E1 05 0A D9 32 DA 34 C6 88 1E}
    $xorCrypt    = {F7 FB 8A 44 0A 04 30 06 FF 41 0C}
    $string_res1 = "loader_id"
    $string_res2 = "born_date"
    $string_res3 = "work_srv"
    $string_res4 = "flags_upd"
    $string_res5 = "lid_file_upd"
    $string_res6 = "localcfg"
    $string_var0 = "%RND_NUM"
    $string_var1 = "%SYS_JR"
    $string_var2 = "%SYS_N"
    $string_var3 = "%SYS_RN"
    $string_var4 = "%RND_SPACE"
    $string_var5 = "%RND_DIGIT"
    $string_var6 = "%RND_HEX"
    $string_var7 = "%RND_hex"
    $string_var8 = "%RND_char"
    $string_var9 = "%RND_CHAR"

  condition:
    (7 of ($string_var*) 
      and 4 of ($string_res*)) 
    or (7 of ($string_var*) 
      and 2 of ($decryptStr, $xorGreet, $xorCrypt)) 
    or (4 of ($string_res*) 
      and 2 of ($decryptStr, $xorGreet, $xorCrypt))
}
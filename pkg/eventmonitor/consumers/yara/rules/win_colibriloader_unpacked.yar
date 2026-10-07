rule win_colibriloader_unpacked : loader
{
  meta:
    author =      "andretavare5"
    description = "ColibriLoader malware"
    org =         "BitSight"
    date =        "2022-11-22"
    md5 =         "f1bbf3a0c6c52953803e5804f4e37b15"
    reference =   "https://malpedia.caad.fkie.fraunhofer.de/details/win.colibri"
    license =     "CC BY-NC-SA 4.0"

	yarahub_author_twitter =    "@andretavare5"
    yarahub_reference_link =    "https://fr3d.hk/blog/colibri-loader-back-to-basics"
    yarahub_malpedia_family =   "win.colibri"
    yarahub_uuid =              "1dcc7399-8e13-4a21-9fec-fb1e08c640a6"
    yarahub_license =           "CC BY-NC-SA 4.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp =  "TLP:WHITE"
    yarahub_reference_md5 =     "f1bbf3a0c6c52953803e5804f4e37b15"
    
  strings:
  	// str decrypt loop
    // --------------------------
    // xor     edx, edx
    // mov     eax, ebx
    // div     [ebp+key_len]
    // mov     ax, [esi+edx*2]
    // xor     ax, [edi+ecx]
    // inc     ebx
    // mov     [ecx], ax
    // lea     ecx, [ecx+2]
    // cmp     ebx, [ebp+str_len]
    // jb      short loc_40596A
    $x = {33 D2 8B C3 F7 75 14 66 8B 04 56 66 33 04 0F 43 66 89 01 8D 49 02 3B 5D 0C 72 E5} 
                              
  condition:
    uint16(0) == 0x5A4D and // MZ
    all of them
}
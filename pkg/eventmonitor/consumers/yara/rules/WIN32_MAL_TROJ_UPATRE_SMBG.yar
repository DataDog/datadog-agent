rule WIN32_MAL_TROJ_UPATRE_SMBG : TROJAN UPATRE
{
    meta:
        description = "Detects UPATRE Trojan variant."
        author = "Auto-generated rule"
        reference = "Not provided"
        sharing = "TLP:WHITE"
        // techniques and mitre_att may not be accurate. As they are based on static analyis.
        // For higher accuracy, Dynamic analysis is needed.
        techniques = "Scripting:Data Obfuscation:Indicator Removal on Host:User Execution:Execution through Module Load:Reflective Loading"
        mitre_att = "T1064:T1001:T1070:T1204:T1129:T1218"
        scan_type = "file"
        dname = "TROJ_UPATRE.SMBG"
        ml_probability_score = "99.03"
        score = 99
        file_path = "/core/upatre/d6067e1501f202563d369a09b40765d56e9be98cdf98214b634eef96abec3bb2"
        sha1_hash = "23cc3f7ade79238ce186ae093fb117a79a286217"
        sha256_hash = "d6067e1501f202563d369a09b40765d56e9be98cdf98214b634eef96abec3bb2"
        timestamp = "2024-01-31T11:14:11.886140"
        date = "2024-01-31"
        yarahub_license = "CC BY 4.0"
        yarahub_uuid = "511175aa-8ed2-4462-832d-85c42079660b"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "7841e2b26f05e82ae5c1576cc9914707"
        malpedia_family = "win.upatre"
    strings:
        $byte_sequence_0 = { 7524578bd34ac1e2028b9db50500008b7b3c8b7c3b78035c3b1c8b0413 } // sample=d6067e1501f202563d369a09b40765d56e9be98cdf98214b634eef96abec3bb2 address=0x00005f32
        /*
                0x00005f32:	7524                	* jne       0x5f58 ; 0x5f58
                0x00005f34:	57                  	* push      edi
                0x00005f35:	8bd3                	* mov       edx, ebx
                0x00005f37:	4a                  	* dec       edx
                0x00005f38:	c1e202              	* shl       edx, 2 ; 0x2
                0x00005f3b:	8b9db5050000        	* mov       ebx, dword ptr [ebp + 0x5b5] ; [0x5b5]
                0x00005f41:	8b7b3c              	* mov       edi, dword ptr [ebx + 0x3c] ; [0x3c]
                0x00005f44:	8b7c3b78            	* mov       edi, dword ptr [ebx + edi + 0x78] ; [0x78]
                0x00005f48:	035c3b1c            	* add       ebx, dword ptr [ebx + edi + 0x1c] ; [0x1c]
                0x00005f4c:	8b0413              	* mov       eax, dword ptr [ebx + edx]
        */
        $byte_sequence_1 = { 8e243b9cac0ac0741432d0b008d1ea730681f29af3a7c1fec875f2ebe7925ac3 } // sample=d6067e1501f202563d369a09b40765d56e9be98cdf98214b634eef96abec3bb2 address=0x00006190
        /*
                0x00006190:	8e243b              	* mov       fs, word ptr [ebx + edi]
                0x00006193:	9c                  	* pushfd    
                0x00006194:	ac                  	* lodsb     al, byte ptr [esi]
                0x00006195:	0ac0                	* or        al, al
                0x00006197:	7414                	* je        0x61ad ; 0x61ad
                0x00006199:	32d0                	* xor       dl, al
                0x0000619b:	b008                	* mov       al, 8 ; 0x8
                0x0000619d:	d1ea                	* shr       edx, 1 ; 0x1
                0x0000619f:	7306                	* jae       0x61a7 ; 0x61a7
                0x000061a1:	81f29af3a7c1        	* xor       edx, 0xc1a7f39a ; 0xc1a7f39a
                0x000061a7:	fec8                	* dec       al
                0x000061a9:	75f2                	* jne       0x619d ; 0x619d
                0x000061ab:	ebe7                	* jmp       0x6194 ; 0x6194
                0x000061ad:	92                  	* xchg      eax, edx
                0x000061ae:	5a                  	* pop       edx
                0x000061af:	c3                  	* ret       
        */
        $byte_sequence_2 = { 03fa8979048b1c96b9180000002bc32bca5fd3e88b4c964403c18b8e88000000 } // sample=d6067e1501f202563d369a09b40765d56e9be98cdf98214b634eef96abec3bb2 address=0x00006669
        /*
                0x00006669:	03fa                	* add       edi, edx
                0x0000666b:	897904              	* mov       dword ptr [ecx + 4], edi ; [0x4]
                0x0000666e:	8b1c96              	* mov       ebx, dword ptr [esi + edx*4]
                0x00006671:	b918000000          	* mov       ecx, 0x18 ; 0x18
                0x00006676:	2bc3                	* sub       eax, ebx
                0x00006678:	2bca                	* sub       ecx, edx
                0x0000667a:	5f                  	* pop       edi
                0x0000667b:	d3e8                	* shr       eax, cl
                0x0000667d:	8b4c9644            	* mov       ecx, dword ptr [esi + edx*4 + 0x44] ; [0x44]
                0x00006681:	03c1                	* add       eax, ecx
                0x00006683:	8b8e88000000        	* mov       ecx, dword ptr [esi + 0x88] ; [0x88]
        */

    condition:
        all of them
}

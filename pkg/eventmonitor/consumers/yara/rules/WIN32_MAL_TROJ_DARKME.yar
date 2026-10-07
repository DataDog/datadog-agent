rule WIN32_MAL_TROJ_DARKME : TROJAN DARKME
{
    meta:
        description = "Detects DARKME trojan variants."
        author = "Jesper Mikkelsen"
        reference = "https://www.trendmicro.com/en_us/research/24/b/cve202421412-water-hydra-targets-traders-with-windows-defender-s.html"
        sharing = "TLP:WHITE"
        // techniques and mitre_att may not be accurate. As they are based on static analyis.
        // For higher accuracy, Dynamic analysis is needed.
        techniques = "Scripting:Data Obfuscation:Template Injection:Reflective Loading"
        mitre_att = "T1064:T1001:T1221:T1218"
        scan_type = "file"
        dname = "Trojan.Win32.DARKME.A"
        ml_probability_score = "98.97"
        score = 98
        file_path = "/core/DARKME/252351cb1fb743379b4072903a5f6c5d29774bf1957defd9a7e19890b3f84146"
        sha1_hash = "0b9a82356134087c4bb62f78496b5461b9fcc572"
        sha256_hash = "252351cb1fb743379b4072903a5f6c5d29774bf1957defd9a7e19890b3f84146"
        timestamp = "2024-02-13T20:19:25.574074"
        date = "2024-02-13"
        yarahub_license = "CC BY 4.0"
        yarahub_uuid = "511175aa-8ed2-4462-832d-85c42079660a"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "3453d05a0acbd06c8774c2ba16644a9f"
    strings:
        $byte_sequence_0 = { 706cf268ffffffff00000000d06d816bb05a314be01c816b5069816b1045 } // sample=252351cb1fb743379b4072903a5f6c5d29774bf1957defd9a7e19890b3f84146 address=0x00001000
        /*
                0x00001000:	706c                	* jo        0x106e ; 0x106e
                0x00001002:	f268ffffffff        	* push      0xffffffff ; 0xffffffff
                0x00001008:	0000                	* add       byte ptr [eax], al
                0x0000100a:	0000                	* add       byte ptr [eax], al
                0x0000100c:	d06d81              	* shr       byte ptr [ebp - 0x7f], 1 ; [0x-7f] ; 0x1
                0x0000100f:	6bb05a314be01c      	* imul      esi, dword ptr [eax - 0x1fb4cea6], 0x1c ; [0x-1fb4cea6] ; 0x1c
                0x00001016:	816b5069816b10      	* sub       dword ptr [ebx + 0x50], 0x106b8169 ; [0x50] ; 0x106b8169
                0x0000101d:	45                  	* inc       ebp
        */
        $byte_sequence_1 = { 816be092816bb01c816b60732c4b9093816b000000000388046612af0f666790 } // sample=252351cb1fb743379b4072903a5f6c5d29774bf1957defd9a7e19890b3f84146 address=0x0000101e
        /*
                0x0000101e:	816be092816bb0      	* sub       dword ptr [ebx - 0x20], 0xb06b8192 ; [0x-20] ; 0xb06b8192
                0x00001025:	1c81                	* sbb       al, 0x81 ; 0x81
                0x00001027:	6b60732c            	* imul      esp, dword ptr [eax + 0x73], 0x2c ; [0x73] ; 0x2c
                0x0000102b:	4b                  	* dec       ebx
                0x0000102c:	90                  	* nop       
                0x0000102d:	93                  	* xchg      eax, ebx
                0x0000102e:	816b0000000003      	* sub       dword ptr [ebx], 0x3000000 ; 0x3000000
                0x00001035:	880466              	* mov       byte ptr [esi], al
                0x00001038:	12af0f666790        	* adc       ch, byte ptr [edi - 0x6f9899f1] ; [0x-6f9899f1]
        */
        $byte_sequence_2 = { 056600460166ef60106651e20066d4681066d3a70f6654f704661b350f665aae } // sample=252351cb1fb743379b4072903a5f6c5d29774bf1957defd9a7e19890b3f84146 address=0x0000103e
        /*
                0x0000103e:	0566004601          	* add       eax, 0x1460066 ; 0x1460066
                0x00001043:	66ef                	* out       dx, ax
                0x00001045:	60                  	* pushal    
                0x00001046:	106651              	* adc       byte ptr [esi + 0x51], ah ; [0x51]
                0x00001049:	e200                	* loop      0x104b ; 0x104b
                0x0000104b:	66d468              	* aam       0x68 ; 0x68
                0x0000104e:	1066d3              	* adc       byte ptr [esi - 0x2d], ah ; [0x-2d]
                0x00001051:	a7                  	* cmpsd     dword ptr [esi], dword ptr es:[edi]
                0x00001052:	0f6654f704          	* pcmpgtd   mm2, qword ptr [edi + esi*8 + 4] ; [0x4]
                0x00001057:	661b350f665aae      	* sbb       si, word ptr [0xae5a660f] ; [0x-51a599f1]
        */
        $byte_sequence_3 = { 0f6623310f6623a30066c227046687a70f666544016672d801661fa80f661f } // sample=252351cb1fb743379b4072903a5f6c5d29774bf1957defd9a7e19890b3f84146 address=0x0000105e
        /*
                0x0000105e:	0f6623              	* pcmpgtd   mm4, qword ptr [ebx]
                0x00001061:	310f                	* xor       dword ptr [edi], ecx
                0x00001063:	6623a30066c227      	* and       sp, word ptr [ebx + 0x27c26600] ; [0x27c26600]
                0x0000106a:	0466                	* add       al, 0x66 ; 0x66
                0x0000106c:	87a70f666544        	* xchg      dword ptr [edi + 0x4465660f], esp ; [0x4465660f]
                0x00001072:	016672              	* add       dword ptr [esi + 0x72], esp ; [0x72]
                0x00001075:	d801                	* fadd      dword ptr [ecx]
                0x00001077:	661f                	* pop       ds
                0x00001079:	a80f                	* test      al, 0xf ; 0xf
                0x0000107b:	661f                	* pop       ds
        */
        $byte_sequence_4 = { a90f6635dd0066ad870466 } // sample=252351cb1fb743379b4072903a5f6c5d29774bf1957defd9a7e19890b3f84146 address=0x0000107d
        /*
                0x0000107d:	a90f6635dd          	* test      eax, 0xdd35660f ; 0xdd35660f
                0x00001082:	0066ad              	* add       byte ptr [esi - 0x53], ah ; [0x-53]
                0x00001085:	870466              	* xchg      dword ptr [esi], eax
        */
        $byte_sequence_5 = { 23570e6678ce03660388046612af0f668769106688830366d6c51066345d } // sample=594e7f7f09a943efc7670edb0926516cfb3c6a0c0036ac1b2370ce3791bf2978 address=0x00001000
        /*
                0x00001000:	23570e              	* and       edx, dword ptr [edi + 0xe] ; [0xe]
                0x00001003:	6678ce              	* js        0xfd4 ; 0xfd4
                0x00001006:	036603              	* add       esp, dword ptr [esi + 3] ; [0x3]
                0x00001009:	880466              	* mov       byte ptr [esi], al
                0x0000100c:	12af0f668769        	* adc       ch, byte ptr [edi + 0x6987660f] ; [0x6987660f]
                0x00001012:	106688              	* adc       byte ptr [esi - 0x78], ah ; [0x-78]
                0x00001015:	830366              	* add       dword ptr [ebx], 0x66 ; 0x66
                0x00001018:	d6                  	* salc      
                0x00001019:	c510                	* lds       edx, ptr [eax]
                0x0000101b:	66345d              	* xor       al, 0x5d ; 0x5d
        */
        $byte_sequence_6 = { 0f6600460166ef6010660d161166d4d901663b9b056651e2006627 } // sample=594e7f7f09a943efc7670edb0926516cfb3c6a0c0036ac1b2370ce3791bf2978 address=0x0000101e
        /*
                0x0000101e:	0f6600              	* pcmpgtd   mm0, qword ptr [eax]
                0x00001021:	46                  	* inc       esi
                0x00001022:	0166ef              	* add       dword ptr [esi - 0x11], esp ; [0x-11]
                0x00001025:	60                  	* pushal    
                0x00001026:	10660d              	* adc       byte ptr [esi + 0xd], ah ; [0xd]
                0x00001029:	16                  	* push      ss
                0x0000102a:	1166d4              	* adc       dword ptr [esi - 0x2c], esp ; [0x-2c]
                0x0000102d:	d901                	* fld       dword ptr [ecx]
                0x0000102f:	663b9b056651e2      	* cmp       bx, word ptr [ebx - 0x1dae99fb] ; [0x-1dae99fb]
                0x00001036:	006627              	* add       byte ptr [esi + 0x27], ah ; [0x27]
        */
        $byte_sequence_7 = { 670566d4681066d3a70f663bff0066de2b1166151911663eda01666dd70166af } // sample=594e7f7f09a943efc7670edb0926516cfb3c6a0c0036ac1b2370ce3791bf2978 address=0x00001039
        /*
                0x00001039:	670566d46810        	* add       eax, 0x1068d466 ; 0x1068d466
                0x0000103f:	66d3a70f663bff      	* shl       word ptr [edi - 0xc499f1], cl ; [0x-c499f1]
                0x00001046:	0066de              	* add       byte ptr [esi - 0x22], ah ; [0x-22]
                0x00001049:	2b11                	* sub       edx, dword ptr [ecx]
                0x0000104b:	66151911            	* adc       ax, 0x1119 ; 0x1119
                0x0000104f:	663eda01            	* fiadd     dword ptr ds:[ecx]
                0x00001053:	666d                	* insw      word ptr es:[edi], dx
                0x00001055:	d7                  	* xlatb     
                0x00001056:	0166af              	* add       dword ptr [esi - 0x51], esp ; [0x-51]
        */
        $byte_sequence_8 = { 0e0f665aae0f66a0d40e66f1d9006634c80e6623a30066b36805665f2804 } // sample=594e7f7f09a943efc7670edb0926516cfb3c6a0c0036ac1b2370ce3791bf2978 address=0x00001059
        /*
                0x00001059:	0e                  	* push      cs
                0x0000105a:	0f665aae            	* pcmpgtd   mm3, qword ptr [edx - 0x52] ; [0x-52]
                0x0000105e:	0f66a0d40e66f1      	* pcmpgtd   mm4, qword ptr [eax - 0xe99f12c] ; [0x-e99f12c]
                0x00001065:	d900                	* fld       dword ptr [eax]
                0x00001067:	6634c8              	* xor       al, 0xc8 ; 0xc8
                0x0000106a:	0e                  	* push      cs
                0x0000106b:	6623a30066b368      	* and       sp, word ptr [ebx + 0x68b36600] ; [0x68b36600]
                0x00001072:	05665f2804          	* add       eax, 0x4285f66 ; 0x4285f66
        */
        $byte_sequence_9 = { 668db70e66a53f0f66c227046687a70f66f714116662660566e1570e666544 } // sample=594e7f7f09a943efc7670edb0926516cfb3c6a0c0036ac1b2370ce3791bf2978 address=0x00001077
        /*
                0x00001077:	668db70e66a53f      	* lea       si, [edi + 0x3fa5660e] ; [0x3fa5660e]
                0x0000107e:	0f66c2              	* pcmpgtd   mm0, mm2
                0x00001081:	27                  	* daa       
                0x00001082:	0466                	* add       al, 0x66 ; 0x66
                0x00001084:	87a70f66f714        	* xchg      dword ptr [edi + 0x14f7660f], esp ; [0x14f7660f]
                0x0000108a:	116662              	* adc       dword ptr [esi + 0x62], esp ; [0x62]
                0x0000108d:	660566e1            	* add       ax, 0xe166 ; 0xe166
                0x00001091:	57                  	* push      edi
                0x00001092:	0e                  	* push      cs
                0x00001093:	666544              	* inc       sp
        */
        $byte_sequence_10 = { 0166ce39046611570e66ee030f } // sample=594e7f7f09a943efc7670edb0926516cfb3c6a0c0036ac1b2370ce3791bf2978 address=0x00001096
        /*
                0x00001096:	0166ce              	* add       dword ptr [esi - 0x32], esp ; [0x-32]
                0x00001099:	390466              	* cmp       dword ptr [esi], eax
                0x0000109c:	11570e              	* adc       dword ptr [edi + 0xe], edx ; [0xe]
                0x0000109f:	66ee                	* out       dx, al
                0x000010a1:	030f                	* add       ecx, dword ptr [edi]
        */
        $byte_sequence_11 = { 89048d44ca0210eb27837d0cfb75146af4ff15c40103108b5508 } // sample=dc1b15e48b68e9670bf3038e095f4afb4b0d8a68b84ae6c05184af7f3f5ecf54 address=0x00001487
        /*
                0x00001487:	89048d44ca0210      	* mov       dword ptr [ecx*4 + 0x1002ca44], eax ; [0x1002ca44]
                0x0000148e:	eb27                	* jmp       0x14b7 ; 0x14b7
                0x00001490:	837d0cfb            	* cmp       dword ptr [ebp + 0xc], -5 ; [0xc] ; -0x5
                0x00001494:	7514                	* jne       0x14aa ; 0x14aa
                0x00001496:	6af4                	* push      -0xc ; -0xc
                0x00001498:	ff15c4010310        	* call      dword ptr [0x100301c4] ; [0x100301c4]
                0x0000149e:	8b5508              	* mov       edx, dword ptr [ebp + 8] ; [0x8]
        */
        $byte_sequence_12 = { c785e4eeffff1c940210eb0ac785e4eeffffcc9102108b45180fbe0885c9740c } // sample=dc1b15e48b68e9670bf3038e095f4afb4b0d8a68b84ae6c05184af7f3f5ecf54 address=0x000019c3
        /*
                0x000019c3:	c785e4eeffff1c940210	* mov       dword ptr [ebp - 0x111c], 0x1002941c ; [0x-111c] ; 0x1002941c
                0x000019cd:	eb0a                	* jmp       0x19d9 ; 0x19d9
                0x000019cf:	c785e4eeffffcc910210	* mov       dword ptr [ebp - 0x111c], 0x100291cc ; [0x-111c] ; 0x100291cc
                0x000019d9:	8b4518              	* mov       eax, dword ptr [ebp + 0x18] ; [0x18]
                0x000019dc:	0fbe08              	* movsx     ecx, byte ptr [eax]
                0x000019df:	85c9                	* test      ecx, ecx
                0x000019e1:	740c                	* je        0x19ef ; 0x19ef
        */
        $byte_sequence_13 = { 740cc785c8eefffffc930210eb0ac785c8eeffffcc9102108b95eceeffff52 } // sample=dc1b15e48b68e9670bf3038e095f4afb4b0d8a68b84ae6c05184af7f3f5ecf54 address=0x00001a8f
        /*
                0x00001a8f:	740c                	* je        0x1a9d ; 0x1a9d
                0x00001a91:	c785c8eefffffc930210	* mov       dword ptr [ebp - 0x1138], 0x100293fc ; [0x-1138] ; 0x100293fc
                0x00001a9b:	eb0a                	* jmp       0x1aa7 ; 0x1aa7
                0x00001a9d:	c785c8eeffffcc910210	* mov       dword ptr [ebp - 0x1138], 0x100291cc ; [0x-1138] ; 0x100291cc
                0x00001aa7:	8b95eceeffff        	* mov       edx, dword ptr [ebp - 0x1114] ; [0x-1114]
                0x00001aad:	52                  	* push      edx
        */

    condition:
        any of them
}

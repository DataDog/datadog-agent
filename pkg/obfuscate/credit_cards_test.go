// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIINValidCardPrefix(t *testing.T) {
	for _, tt := range []struct {
		in         int
		maybe, yes bool
	}{
		// yes
		{1, false, true},
		{4, false, true},
		// maybe
		{2, true, false},
		{3, true, false},
		{5, true, false},
		{6, true, false},
		// no
		{7, false, false},
		{8, false, false},
		{9, false, false},

		// yes
		{34, false, true},
		{37, false, true},
		{39, false, true},
		{51, false, true},
		{55, false, true},
		{62, false, true},
		{65, false, true},
		// maybe
		{30, true, false},
		{63, true, false},
		{22, true, false},
		{27, true, false},
		{69, true, false},
		// no
		{31, false, false},
		{29, false, false},
		{21, false, false},

		// yes
		{300, false, true},
		{305, false, true},
		{644, false, true},
		{649, false, true},
		{309, false, true},
		{636, false, true},
		// maybe
		{352, true, false},
		{358, true, false},
		{501, true, false},
		{601, true, false},
		{222, true, false},
		{272, true, false},
		{500, true, false},
		{509, true, false},
		{560, true, false},
		{589, true, false},
		{600, true, false},
		{699, true, false},

		// yes
		{3528, false, true},
		{3589, false, true},
		{5019, false, true},
		{6011, false, true},
		// maybe
		{2221, true, false},
		{2720, true, false},
		{5000, true, false},
		{5099, true, false},
		{5600, true, false},
		{5899, true, false},
		{6000, true, false},
		{6999, true, false},

		// maybe
		{22210, true, false},
		{27209, true, false},
		{50000, true, false},
		{50999, true, false},
		{56000, true, false},
		{58999, true, false},
		{60000, true, false},
		{69999, true, false},
		// no
		{21000, false, false},
		{55555, false, false},

		// yes
		{222100, false, true},
		{272099, false, true},
		{500000, false, true},
		{509999, false, true},
		{560000, false, true},
		{589999, false, true},
		{600000, false, true},
		{699999, false, true},
		// no
		{551234, false, false},
		{594388, false, false},
		{219899, false, false},
	} {
		t.Run(fmt.Sprintf("%d", tt.in), func(t *testing.T) {
			maybe, yes := validCardPrefix(tt.in)
			assert.Equal(t, maybe, tt.maybe)
			assert.Equal(t, yes, tt.yes)
		})
	}
}

func TestIINIsSensitive(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for i, valid := range []string{
			"378282246310005",
			"  378282246310005",
			"  3782-8224-6310-005 ",
			"371449635398431",
			"378734493671000",
			"5610591081018250",
			"30569309025904",
			"38520000023237",
			"6011 1111 1111 1117",
			"6011000990139424",
			" 3530111333--300000  ",
			"3566002020360505",
			"5555555555554444",
			"5105-1051-0510-5100",
			" 4111111111111111",
			"4012888888881881 ",
			"422222 2222222",
			"5019717010103742",
			"6331101999990016",
			" 4242-4242-4242-4242 ",
			"4242-4242-4242-4242 ",
			"4242-4242-4242-4242  ",
			"4000056655665556",
			"5555555555554444",
			"2223003122003222",
			"5200828282828210",
			"5105105105105100",
			"378282246310005",
			"371449635398431",
			"6011111111111117",
			"6011000990139424",
			"3056930009020004",
			"3566002020360505",
			"620000000000000",
			"2222 4053 4324 8877",
			"2222 9909 0525 7051",
			"2223 0076 4872 6984",
			"2223 5771 2001 7656",
			"5105 1051 0510 5100",
			"5111 0100 3017 5156",
			"5185 5408 1000 0019",
			"5200 8282 8282 8210",
			"5204 2300 8000 0017",
			"5204 7400 0990 0014",
			"5420 9238 7872 4339",
			"5455 3307 6000 0018",
			"5506 9004 9000 0436",
			"5506 9004 9000 0444",
			"5506 9005 1000 0234",
			"5506 9208 0924 3667",
			"5506 9224 0063 4930",
			"5506 9274 2731 7625",
			"5553 0422 4198 4105",
			"5555 5537 5304 8194",
			"5555 5555 5555 4444",
			"4012 8888 8888 1881",
			"4111 1111 1111 1111",
			"6011 0009 9013 9424",
			"6011 1111 1111 1117",
			"3714 496353 98431",
			"3782 822463 10005",
			"3056 9309 0259 04",
			"3852 0000 0232 37",
			"3530 1113 3330 0000",
			"3566 0020 2036 0505",
			"3700 0000 0000 002",
			"3700 0000 0100 018",
			"6703 4444 4444 4449",
			"4871 0499 9999 9910",
			"4035 5010 0000 0008",
			"4360 0000 0100 0005",
			"6243 0300 0000 0001",
			"5019 5555 4444 5555",
			"3607 0500 0010 20",
			"6011 6011 6011 6611",
			"6445 6445 6445 6445",
			"5066 9911 1111 1118",
			"6062 8288 8866 6688",
			"3569 9900 1009 5841",
			"6771 7980 2100 0008",
			"2222 4000 7000 0005",
			"5555 3412 4444 1115",
			"5577 0000 5577 0004",
			"5555 4444 3333 1111",
			"2222 4107 4036 0010",
			"5555 5555 5555 4444",
			"2222 4107 0000 0002",
			"2222 4000 1000 0008",
			"2223 0000 4841 0010",
			"2222 4000 6000 0007",
			"2223 5204 4356 0010",
			"2222 4000 3000 0004",
			"5100 0600 0000 0002",
			"2222 4000 5000 0009",
			"1354 1001 4004 955",
			"4111 1111 4555 1142",
			"4988 4388 4388 4305",
			"4166 6766 6766 6746",
			"4646 4646 4646 4644",
			"4000 6200 0000 0007",
			"4000 0600 0000 0006",
			"4293 1891 0000 0008",
			"4988 0800 0000 0000",
			"4111 1111 1111 1111",
			"4444 3333 2222 1111",
			"4001 5900 0000 0001",
			"4000 1800 0000 0002",
			"4000 0200 0000 0000",
			"4000 1600 0000 0004",
			"4002 6900 0000 0008",
			"4400 0000 0000 0008",
			"4484 6000 0000 0004",
			"4607 0000 0000 0009",
			"4977 9494 9494 9497",
			"4000 6400 0000 0005",
			"4003 5500 0000 0003",
			"4000 7600 0000 0001",
			"4017 3400 0000 0003",
			"4005 5190 0000 0006",
			"4131 8400 0000 0003",
			"4035 5010 0000 0008",
			"4151 5000 0000 0008",
			"4571 0000 0000 0001",
			"4199 3500 0000 0002",
			"4001 0200 0000 0009",
		} {
			cco := &creditCard{luhn: true}
			t.Run("", func(t *testing.T) {
				assert.True(t, cco.IsCardNumber(valid), i)
			})
		}
	})

	t.Run("invalid", func(t *testing.T) {
		cco := &creditCard{luhn: false}
		for i, invalid := range []string{
			"37828224631000521389798", // valid but too long
			"37828224631",             // valid but too short
			"   3782822-4631 ",
			"3714djkkkksii31",  // invalid character
			"x371413321323331", // invalid characters
			"",
			"7712378231899",
			"   -  ",
		} {
			assert.False(t, cco.IsCardNumber(invalid), i)
		}
	})
}

func BenchmarkIsSensitive(b *testing.B) {
	run := func(str string, luhn bool) func(b *testing.B) {
		cco := &creditCard{luhn: luhn}
		return func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				cco.IsCardNumber(str)
			}
		}
	}

	b.Run("basic", run("4001 0200 0000 0009", false))
	b.Run("luhn", run("4001 0200 0000 0009", true))
}

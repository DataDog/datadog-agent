package pb

import (
	fmt "fmt"
	"testing"
)

var (
	AAA = APMSampling2{
		TargetTps: []TargetTPS2{
			{
				"banana",
				"pineaple",
				3,
			},
			{
				"bean",
				"pam",
				3,
			},
		},
	}
	BBB = APMSampling{
		TargetTps: []TargetTPS{
			{
				"banana",
				"pineaple",
				3,
			},
			{
				"bean",
				"pam",
				3,
			},
		},
	}
)

func TestSizes(t *testing.T) {

	msg, _ := AAA.MarshalMsg(nil)
	fmt.Println("msgp - interned", len(msg))

	msg2, _ := BBB.MarshalMsg(nil)
	fmt.Println("msgp - default", len(msg2))

	msg3, _ := BBB.Marshal()
	fmt.Println("proto", len(msg3))
}

func BenchmarkMarshalRemoteMsgpInterned(b *testing.B) {
	res := make([]byte, 0, 500)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AAA.MarshalMsg(res)
		res = res[0:]
	}
}

func BenchmarkMarshalRemoteMsgpDefault(b *testing.B) {
	b.StopTimer()
	res := make([]byte, 0, 500)
	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		BBB.MarshalMsg(res)
		res = res[0:]
	}
}

func BenchmarkMarshalRemoteProto(b *testing.B) {
	b.StopTimer()
	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		BBB.Marshal()
	}
}

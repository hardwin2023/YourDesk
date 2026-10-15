package video

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func hevcTestNAL(kind byte) []byte { return []byte{kind << 1, 1, 0x80} }
func hevcTestAnnex(nals ...[]byte) []byte {
	var out []byte
	for _, nal := range nals {
		out = append(out, 0, 0, 0, 1)
		out = append(out, nal...)
	}
	return out
}
func hevcTestLengths(nals ...[]byte) []byte {
	var out []byte
	for _, nal := range nals {
		out = binary.BigEndian.AppendUint32(out, uint32(len(nal)))
		out = append(out, nal...)
	}
	return out
}
func hevcTestHVCC(sets ...[]byte) []byte {
	out := make([]byte, 23)
	out[0] = 1
	out[21] = 3
	out[22] = byte(len(sets))
	for _, nal := range sets {
		out = append(out, (nal[0]>>1)&63, 0, 1)
		out = binary.BigEndian.AppendUint16(out, uint16(len(nal)))
		out = append(out, nal...)
	}
	return out
}

func TestPackHEVCFormatsAndGOP(t *testing.T) {
	vps, sps, pps, idr := hevcTestNAL(32), hevcTestNAL(33), hevcTestNAL(34), hevcTestNAL(19)
	want := hevcTestAnnex(vps, sps, pps, idr)
	for _, tc := range []struct {
		name          string
		frame, config []byte
	}{
		{"Annex B 帶參數集", want, nil},
		{"Annex B 分開參數集", hevcTestAnnex(idr), hevcTestAnnex(vps, sps, pps)},
		{"長度前綴", hevcTestLengths(idr), hevcTestAnnex(vps, sps, pps)},
		{"hvcC", hevcTestLengths(idr), hevcTestHVCC(vps, sps, pps)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sets hevcParameterSets
			packed, err := packHEVC(tc.frame, tc.config, &sets)
			if err != nil {
				t.Fatal(err)
			}
			key, err := IsKeyframe(WireHEVC, packed)
			if err != nil || !key {
				t.Fatal("首張不是 IDR", err)
			}
			annex, err := unpackHEVC(packed)
			if err != nil || !bytes.Equal(annex, want) {
				t.Fatalf("跨平台封包不一致：%x %v", annex, err)
			}
			delta := hevcTestNAL(1)
			packed, err = packHEVC(hevcTestLengths(delta), nil, &sets)
			if err != nil {
				t.Fatal("GOP 未沿用參數集", err)
			}
			key, err = IsKeyframe(WireHEVC, packed)
			if err != nil || key {
				t.Fatal("參考影格被誤當 IDR", err)
			}
			annex, err = unpackHEVC(packed)
			if err != nil || !bytes.Equal(annex, hevcTestAnnex(vps, sps, pps, delta)) {
				t.Fatal("參考影格無法解封", err)
			}
		})
	}
}

func TestPackHEVCRealFixture(t *testing.T) {
	annex, err := unpackHEVC(probeHEVC)
	if err != nil {
		t.Fatal(err)
	}
	var sets hevcParameterSets
	packed, err := packHEVC(annex, nil, &sets)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := unpackHEVC(packed)
	if err != nil || !bytes.Equal(actual, annex) {
		t.Fatal("真實 HEVC 樣本封裝後改變", err)
	}
	key, err := IsKeyframe(WireHEVC, packed)
	if err != nil || !key {
		t.Fatal("真實樣本未識別 IDR", err)
	}
}

func TestPackHEVCValidationAndOwnership(t *testing.T) {
	config := hevcTestAnnex(hevcTestNAL(32), hevcTestNAL(33), hevcTestNAL(34))
	frame := hevcTestLengths(hevcTestNAL(19))
	var sets hevcParameterSets
	packed, err := packHEVC(frame, config, &sets)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(packed)
	config[6] ^= 1
	frame[len(frame)-1] ^= 1
	if !bytes.Equal(packed, want) || sets[0][2] != 0x80 {
		t.Fatal("封包或快取指向可變的輸入資料")
	}
	for _, tc := range []struct {
		name          string
		frame, config []byte
	}{
		{"缺少影像", hevcTestAnnex(hevcTestNAL(32)), nil},
		{"截斷 NAL", []byte{0, 0, 0, 2, 0x26}, nil},
		{"空 NAL", []byte{0, 0, 0, 0}, nil},
		{"無效標頭", []byte{0, 0, 0, 2, 0x26, 0}, nil},
		{"hvcC 截斷", hevcTestLengths(hevcTestNAL(19)), []byte{1, 0}},
		{"hvcC 尾端多資料", hevcTestLengths(hevcTestNAL(19)), append(hevcTestHVCC(hevcTestNAL(32)), 1)},
		{"NAL 過多", bytes.Repeat(hevcTestLengths(hevcTestNAL(1)), maxVideoNALs), nil},
		{"設定過大", hevcTestLengths(hevcTestNAL(19)), make([]byte, 65537)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := sets
			if _, err := packHEVC(tc.frame, tc.config, &sets); err == nil {
				t.Fatal("無效封包未拒絕")
			}
			for i := range sets {
				if !bytes.Equal(sets[i], previous[i]) {
					t.Fatal("錯誤改寫了參數快取")
				}
			}
		})
	}
	if _, err := packHEVC(hevcTestLengths(hevcTestNAL(19)), nil, nil); err == nil {
		t.Fatal("新工作階段缺少參數集未拒絕")
	}
}

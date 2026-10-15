package video

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type hevcParameterSets [3][]byte

func validHEVCNAL(nal []byte) bool {
	return len(nal) >= 2 && nal[0]&0x80 == 0 && nal[1]&7 != 0
}

func hevcLengthNALs(data []byte) ([][]byte, error) {
	var nals [][]byte
	for len(data) > 0 {
		if len(nals) >= maxVideoNALs || len(data) < 4 {
			return nil, fmt.Errorf("HEVC NAL 數量或長度無效")
		}
		n := uint64(binary.BigEndian.Uint32(data))
		data = data[4:]
		if n > uint64(len(data)) || !validHEVCNAL(data[:int(n)]) {
			return nil, fmt.Errorf("HEVC NAL 長度或標頭無效")
		}
		nals = append(nals, data[:int(n)])
		data = data[int(n):]
	}
	if len(nals) == 0 {
		return nil, fmt.Errorf("HEVC 沒有 NAL")
	}
	return nals, nil
}

func hevcAnnexBNALs(data []byte) ([][]byte, error) {
	start := func(from int) (int, int) {
		for i := from; i+3 <= len(data); i++ {
			if data[i] == 0 && data[i+1] == 0 {
				if data[i+2] == 1 {
					return i, 3
				}
				if i+4 <= len(data) && data[i+2] == 0 && data[i+3] == 1 {
					return i, 4
				}
			}
		}
		return -1, 0
	}
	first, prefix := start(0)
	if first < 0 {
		return nil, fmt.Errorf("HEVC 缺少 Annex B 起始碼")
	}
	for _, c := range data[:first] {
		if c != 0 {
			return nil, fmt.Errorf("HEVC 起始碼前資料無效")
		}
	}
	var nals [][]byte
	for at, n := first, prefix; at >= 0; {
		if len(nals) >= maxVideoNALs {
			return nil, fmt.Errorf("HEVC NAL 過多")
		}
		next, np := start(at + n)
		end := next
		if end < 0 {
			end = len(data)
		}
		for end > at+n && data[end-1] == 0 {
			end--
		}
		nal := data[at+n : end]
		if !validHEVCNAL(nal) {
			return nil, fmt.Errorf("HEVC NAL 截斷或標頭無效")
		}
		nals = append(nals, nal)
		at, n = next, np
	}
	return nals, nil
}

func hevcNALs(data []byte) ([][]byte, error) {
	if nals, err := hevcLengthNALs(data); err == nil {
		return nals, nil
	}
	return hevcAnnexBNALs(data)
}

// MF 的序列標頭使用 Annex B；亦接收第三方 MFT 的 hvcC 設定。
func hevcConfig(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] != 1 {
		return hevcNALs(data)
	}
	if len(data) < 23 || data[21]&3 != 3 {
		return nil, fmt.Errorf("不支援的 HEVC hvcC")
	}
	arrays := int(data[22])
	data = data[23:]
	var nals [][]byte
	for i := 0; i < arrays; i++ {
		if len(data) < 3 {
			return nil, fmt.Errorf("HEVC hvcC 截斷")
		}
		kind := data[0] & 63
		count := int(binary.BigEndian.Uint16(data[1:3]))
		data = data[3:]
		if count > maxVideoNALs-len(nals) {
			return nil, fmt.Errorf("HEVC NAL 過多")
		}
		for j := 0; j < count; j++ {
			if len(data) < 2 {
				return nil, fmt.Errorf("HEVC hvcC NAL 截斷")
			}
			n := int(binary.BigEndian.Uint16(data))
			data = data[2:]
			if n > len(data) || !validHEVCNAL(data[:n]) || (data[0]>>1)&63 != kind {
				return nil, fmt.Errorf("HEVC hvcC NAL 無效")
			}
			nals = append(nals, data[:n])
			data = data[n:]
		}
	}
	if len(data) != 0 {
		return nil, fmt.Errorf("HEVC hvcC 尾端資料無效")
	}
	return nals, nil
}

// 統一 Windows 的 Annex B／長度前綴輸出為既有跨平台 VPS、SPS、PPS wire 格式。
// GOP 的參考影格可沿用上一份已驗證參數集；失敗不改變快取。
func packHEVC(data, config []byte, cache *hevcParameterSets) ([]byte, error) {
	if len(data) > 32<<20 || len(config) > 65536 {
		return nil, fmt.Errorf("HEVC 影格過大")
	}
	nals, err := hevcNALs(data)
	if len(config) > 0 && config[0] == 1 {
		nals, err = hevcLengthNALs(data)
	}
	if err != nil {
		return nil, err
	}
	sets, err := hevcConfig(config)
	if err != nil {
		return nil, err
	}
	var parameters hevcParameterSets
	if cache != nil {
		parameters = *cache
	}
	for _, group := range [][][]byte{sets, nals} {
		for _, nal := range group {
			kind := (nal[0] >> 1) & 63
			if kind >= 32 && kind <= 34 {
				parameters[kind-32] = nal
			}
		}
	}
	size, count, picture := 1, 3, false
	for _, nal := range parameters {
		if len(nal) == 0 {
			return nil, fmt.Errorf("硬體編碼器未提供完整 VPS／SPS／PPS")
		}
		size += 4 + len(nal)
	}
	for _, nal := range nals {
		kind := (nal[0] >> 1) & 63
		if kind >= 32 && kind <= 34 {
			continue
		}
		picture = picture || kind <= 31
		size += 4 + len(nal)
		count++
	}
	if !picture || size > 32<<20 || count > maxVideoNALs {
		return nil, fmt.Errorf("HEVC 影格缺少影像或超過限制")
	}
	out := make([]byte, 1, size)
	out[0] = 3
	appendNAL := func(nal []byte) {
		out = binary.BigEndian.AppendUint32(out, uint32(len(nal)))
		out = append(out, nal...)
	}
	for _, nal := range parameters {
		appendNAL(nal)
	}
	for _, nal := range nals {
		kind := (nal[0] >> 1) & 63
		if kind < 32 || kind > 34 {
			appendNAL(nal)
		}
	}
	if cache != nil {
		for i, nal := range parameters {
			if !bytes.Equal((*cache)[i], nal) {
				(*cache)[i] = bytes.Clone(nal)
			}
		}
	}
	return out, nil
}

// 將既有 VPS／SPS／PPS 封包轉成 Media Foundation 使用的 Annex B。
func unpackHEVC(data []byte) ([]byte, error) {
	if _, err := IsKeyframe(WireHEVC, data); err != nil {
		return nil, err
	}
	data = data[1:]
	out := make([]byte, 0, len(data))
	index := 0
	for len(data) > 0 {
		n := int(binary.BigEndian.Uint32(data))
		data = data[4:]
		if n < 2 {
			return nil, fmt.Errorf("HEVC NAL 截斷")
		}
		kind := (data[0] >> 1) & 63
		if index < 3 && kind != byte(32+index) {
			return nil, fmt.Errorf("HEVC 參數集無效")
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, data[:n]...)
		data = data[n:]
		index++
	}
	return out, nil
}

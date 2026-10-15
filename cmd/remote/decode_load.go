package main

import (
	"sync/atomic"
	"time"
)

const slowDecodeLimit = 120 * time.Millisecond
const slowDecodeSamples = 3

type decodeHistory struct {
	warmed bool
	drops  uint64
	slow   int
}

// 每個連線獨立。網路回呼只累計丟幀，單一解碼工作者判斷實際耗時；
// 不以來源硬編／接收軟解的組合本身判定不支援。
type decodeLoad struct {
	drops    [4]atomic.Uint64
	rejected atomic.Uint32
	history  [4]decodeHistory
}

func (s *decodeLoad) dropped(codec byte) {
	if codec > 0 && codec < byte(len(s.drops)) {
		s.drops[codec].Add(1)
	}
}

func (s *decodeLoad) blocked(codec byte) bool {
	return codec > 0 && codec < byte(len(s.drops)) && s.rejected.Load()&(1<<codec) != 0
}

// 冷啟動不計入；持續解碼超過 120 ms 且每次都有新的滿載丟幀，
// 才在本次連線撤回該格式。健康、閒置或短暫尖峰會中斷連續計數。
func (s *decodeLoad) observe(codec byte, mode string, elapsed time.Duration) bool {
	if codec == 0 || codec >= byte(len(s.history)) || (mode != "software" && mode != "hardware") || s.blocked(codec) {
		return false
	}
	h := &s.history[codec]
	drops := s.drops[codec].Load()
	pressure := drops > h.drops
	h.drops = drops
	if !h.warmed {
		h.warmed = true
		return false
	}
	if elapsed < slowDecodeLimit || !pressure {
		h.slow = 0
		return false
	}
	h.slow++
	if h.slow < slowDecodeSamples {
		return false
	}
	s.rejected.Or(1 << codec)
	return true
}

func (s *decodeLoad) available(codecs []byte) []byte {
	result := make([]byte, 0, len(codecs))
	for _, codec := range codecs {
		if !s.blocked(codec) {
			result = append(result, codec)
		}
	}
	return result
}

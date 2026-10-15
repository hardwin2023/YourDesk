package main

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
	"yourdesk/internal/streampipeline"
)

func TestSlowDecodeRequiresSustainedQueuePressure(t *testing.T) {
	for _, mode := range []string{"software", "hardware"} {
		t.Run(mode, func(t *testing.T) {
			var s decodeLoad
			s.dropped(2)
			if s.observe(2, mode, time.Second) {
				t.Fatal("冷啟動被判為過載")
			}
			for range 20 {
				if s.observe(2, mode, time.Second) {
					t.Fatal("沒有新丟幀卻停用慢速格式")
				}
			}
			for range 2 {
				s.dropped(2)
				if s.observe(2, mode, time.Second) {
					t.Fatal("不足三次就判定過載")
				}
			}
			s.dropped(2)
			if s.observe(2, mode, 10*time.Millisecond) {
				t.Fatal("健康影格誤判")
			}
			for range slowDecodeSamples - 1 {
				s.dropped(2)
				if s.observe(2, mode, time.Second) {
					t.Fatal("健康影格未中斷連續計數")
				}
			}
			s.dropped(2)
			if !s.observe(2, mode, time.Second) || !s.blocked(2) {
				t.Fatal("持續過載沒有撤回該格式")
			}
			input := []byte{1, 2, 3}
			if got := s.available(input); !reflect.DeepEqual(got, []byte{1, 3}) || !reflect.DeepEqual(input, []byte{1, 2, 3}) {
				t.Fatal("改動共享能力清單，或錯誤排除其他格式", got, input)
			}
			if s.observe(2, mode, time.Second) {
				t.Fatal("重複觸發相同格式的切換")
			}
			var next decodeLoad
			if next.blocked(2) {
				t.Fatal("永久改寫了下一次連線偏好")
			}
		})
	}
}

func TestJPEGAndUnknownDecodeModesAreNotRejected(t *testing.T) {
	var s decodeLoad
	for range 20 {
		for _, codec := range []byte{0, 1, 2, 3, 255} {
			s.dropped(codec)
			if s.observe(codec, "unknown", time.Second) {
				t.Fatal("未知模式判為過載")
			}
		}
		s.dropped(0)
		if s.observe(0, "software", time.Second) {
			t.Fatal("撤回了相容 JPEG 備援")
		}
	}
}

// 60 FPS 來源搭配 300 ms 軟解，使用正式雙槽交接。確認不用等來源
// 停止，就能偵測壓力並讓後續的 JPEG 備援繼續處理。
func TestContinuousSlowDecodeFallsBackWithoutGrowingQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var s decodeLoad
		var jpegFrames atomic.Int32
		consumer := streampipeline.NewConsumer(ctx, func(codec byte) {
			if s.blocked(codec) {
				return
			}
			if codec == 0 {
				jpegFrames.Add(1)
				return
			}
			start := time.Now()
			time.Sleep(300 * time.Millisecond)
			s.observe(codec, "software", time.Since(start))
		})
		ticker := time.NewTicker(time.Second / 60)
		defer ticker.Stop()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for range 600 {
				<-ticker.C
				codec := byte(2)
				if s.blocked(codec) {
					codec = 0
				}
				if !consumer.TrySubmit(codec) {
					s.dropped(codec)
				}
			}
		}()
		<-done
		synctest.Wait()
		consumer.Close()
		if !s.blocked(2) || jpegFrames.Load() < 400 {
			t.Fatalf("未自動恢復備援串流：JPEG=%d", jpegFrames.Load())
		}
	})
}

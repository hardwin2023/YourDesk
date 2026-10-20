# 合成影音 Smoke 素材

音訊素材由 FFmpeg 產生 1 秒、440 Hz 的雙聲道合成訊號，不包含錄音或使用者內容。此目錄的素材僅封裝到 androidTest APK，不放入正式 APK。

- `audio-opus.bin`：48 kHz、20 ms、96 kbps，使用 `libopus -application audio -frame_duration 20`；移除 Ogg 封裝及 OpusHead／OpusTags。
- `audio-aac.bin`：48 kHz、AAC-LC、128 kbps，使用 `aac -profile:a aac_low`；移除 ADTS 標頭。
- 共同輸入為 `sine=frequency=440:sample_rate=48000:duration=1`、`volume=0.35`、雙聲道。
- 檔案結構為重複的「4 bytes big-endian 封包長度＋單一壓縮音訊封包」，沒有 YDA1 標頭；協定 Host 會加入目前世代／序號。
- PCM 由 Smoke Host 即時產生，同樣使用 48 kHz／16-bit／雙聲道。

`video-h264-1080.json` 與 `video-hevc-1080.json` 是以正式桌面 VideoToolbox 編碼器產生的 1920×1080 合成移動色塊，每份 60 幀、60 FPS、4 Mbps、GOP 30，包含 IDR 與相依幀。JSON 記錄格式、尺寸及逐幀 wire payload（Base64）；沒有使用者桌面內容，也不封裝到正式 APK。需有對應硬體編碼器的 macOS 執行：

```sh
go run ./android/tests/video-fixtures
```

測試 Host 的 `smoke.video.h264`／`smoke.video.hevc` 指令開始重播；`smoke.video.drop` 遺失單一相依幀，`video.keyframe` 從 IDR 恢復，撤回能力後回到 JPEG。手機測試以 TextureView 實際更新回呼、UI 心跳與往返控制屏障驗證，不以接收封包數替代呈現成功。

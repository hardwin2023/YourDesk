# Android 實機串流驗證（2026-10-20）

## 環境與範圍

- 裝置：ASUS_I01WD，Android 11／API 30，arm64，4 KB 記憶體分頁。
- 來源：`2c15f02832da1fe655eb1326181d241f5f931fd9` 加上本次實機修正；驗證 10 月 15 日新增的背景解碼、有界交接、原生停滯隔離與輸入工作者。
- 使用獨立 `com.yourdesk.android.debug`／測試 APK，不覆蓋手機的正式 `com.yourdesk.android`。正式版原為 `1.26.1001 build 1524`。
- 手機經 Wi-Fi 與 Mac 上的隔離測試 Host 連線，走正式 TLS、WebRTC／P2P、JNI、MediaCodec、TextureView 與控制管線。Host 重播合成移動色塊與測試音，不讀取使用者桌面、密碼或站台，也不向作業系統注入輸入。
- 影像來源為 1920×1080、60 FPS、GOP 30 的 H.264／HEVC，每份素材包含 IDR 與相依幀。素材以正式 VideoToolbox 編碼器產生；4 Mbps 是產生時設定的碼率上限，合成內容的實際碼率較低。

## 發現與修正

1. **硬體初始化被過載重設反覆取消。** 舊背景交接在滿載或缺幀時銷毀 MediaCodec；真機初始化長於兩個影格間隔，造成建立、銷毀循環。初次網路 H.264 驗證收到影格，卻沒有 TextureView 呈現回呼。現將缺幀恢復與 Surface／工作階段重建分開，保留配置並等待新的 IDR。
2. **缺幀通知造成重複恢復。** 等待關鍵影格時，每張被丟棄的相依幀可能重複通知同一缺口；現只處理一次。支援 Surface 自適應播放的候選直接接續 IDR，不反覆 flush 有效輸出；不支援者仍 flush，首張輸出前的 flush 另補送 CSD。依據 [MediaCodec 的自適應播放與串流邊界約定](https://developer.android.com/reference/android/media/MediaCodec)，不是按手機型號特判。
3. **立即重連偶發 HTTP 409。** 前一工作階段尚未清理，Host 暫時拒絕新的直連。桌面與 Android 直連客戶端在原十秒期限內，每 200 ms 重試 409；取消、其他錯誤與 TLS／密碼驗證保留。Go Smoke 驗證重試後仍綁定該次 TLS exporter。
4. **舊裝置測試仍把 queue 當同步呈現。** 本機 H.264 fixture 改為等待背景輸出並按需求補送 IDR；像素、座標與螢幕切換斷言保留。自動點擊另透過 Gradle 的 `animationsDisabled` 選項排除系統旋轉動畫的暫時座標變換，不修改產品輸入邏輯。

## 實測結果

完整裝置回歸 **42 項全數通過，0 失敗、0 錯誤、0 略過**：聲音 3 項、畫面／操作 17 項、網路整合 11 項、頁面與相機 3 項、影像合成恢復 2 項、站台資料 5 項、原生 H.264 解碼 1 項。完整回歸內的影片案例另驗證遺失影格確實改變恢復世代，之後才等待新畫面，並完成八次立即直連重連。

兩種格式在持續階段皆通過，沒有觸發連續三秒無更新、UI 超時或硬解能力撤回。以 `TextureView.onSurfaceTextureUpdated` 計算實際呈現，不以來源 60 FPS 宣稱手機全程達到 60 FPS。

| 格式 | 實際硬體後端 | 持續時間 | 持續階段呈現張數 | 平均呈現 FPS | 最慢 UI 心跳 | 最慢控制屏障往返 |
| --- | --- | --- | --- | --- | --- | --- |
| H.264 | `OMX.qcom.video.decoder.avc` | 180.534 秒 | 7,119 | 39.43 | 17 ms | 128 ms |
| HEVC | `OMX.qcom.video.decoder.hevc` | 180.598 秒 | 8,516 | 47.15 | 18 ms | 122 ms |

每種格式另完成相依幀遺失、進入背景及回到前景後的新影格呈現。影片仍在輸出時，透過正式關閉按鈕返回首頁需 140 ms，隨後立即重連並重新呈現 H.264 成功。控制屏障往返包含傳輸與 Host 控制工作者，不代表完整的桌面擷取到使用者看見滑鼠結果的延遲。

其他本機檢查：25 項 JUnit 通過；10,630 個解析／MediaCodec API 替身斷言與非同步工作者 Smoke 通過，包含慢初始化、重複恢復、Surface 重設優先權、停滯與退出。桌面 signaling 及 Android core 的 Go／race 通過；AAR 來源指紋、個人路徑及 16 KB ELF 檢查通過；Debug／未簽章 Release APK 編譯、Release lint 與 APK 原生 ELF／ZIP 對齊檢查通過。lint 沒有 error／fatal，仍有 31 項 warning。

本機證據保存於 `.local-run/android-device-20261020/`（不提交裝置日誌或測試密碼）：

- `video-adaptive-results.xml`、`video-adaptive-logcat.txt`、`video-device-smoke.json`：三分鐘串流、控制、背景恢復、關閉與重連。
- `stream-h264.png`、`stream-hevc-steady.png`：合成畫面與工具列實際截圖。
- `video-host-final.log`、`signaling-regression.log`、`android-core-regression.log`、`verified-build.log`：工作者、Go／race 與建置結果。
- `source-hashes.json`：本次實測的產品來源檔案 SHA-256。
- `full-device-regression-results.xml`、`full-device-regression-video-logcat.txt`：完整 42 項裝置回歸；該輪兩種影片各持續 20 秒，另有上述各 180 秒的獨立延長測試。
- `cleanup.json`：原正式版仍為 build 1524、UID 10213；移除本次 Debug 套件、還原原有動畫設定、關閉測試 Host 並移除拋棄式密碼。原正式套件與既有測試套件保留。

## 可重現方式

依 [Android 建置與 Smoke 指令](ANDROID-BUILD.md#smoke-測試)啟動測試 Host，提供拋棄式 `smokeSecret`，並指定 `videoSoakSeconds=180`。沒有 Host 時網路案例會略過；驗收必須確認沒有略過。兩種硬體解碼器缺一則影片驗收明確失敗，不改用 JPEG 冒充硬解。

影片測試每秒量測 UI 心跳及有序輸入屏障的控制往返，以 TextureView 實際更新回呼計數；另外故意遺失一張相依幀、進入背景再返回、串流中關閉及立即重連。每種格式持續階段不得連續三次秒級檢查沒有更新，且平均至少 15 張／秒、UI 回應不超過一秒、控制往返不超過兩秒。這些是測試失敗門檻，實際量測值另記錄。

## 驗證界線

本次是單一 Android 11 裝置與隔離 LAN Host 的驗證，不能代替 Windows 使用者原始故障現場、桌面擷取／鍵鼠注入、其他晶片、Android 16、16 KB 實機、行動網路切換或數小時負載測試。音訊驗證解碼內容與 AudioTrack 播放進度，未以麥克風回錄。此實機測試階段未改版號或發布；相同產品來源隨後納入 [1.26.1020 build 1252](RELEASE-1.26.1020-build-1252.md)，正式套件另完成簽章與封裝驗證。

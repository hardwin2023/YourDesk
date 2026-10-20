# 串流過載與安全退出

更新日期：2026-10-20。適用 Windows／macOS 桌面 Viewer 與 Android Viewer 的影片串流。目的在於限制排隊與等待，把原生驅動停滯和 UI 生命週期分開；不因「來源硬編、接收軟解」的組合本身就停用編碼。

## 各平台處理

| 平台 | 原生影片路徑 | 等待與退出 | 輸入交接 |
| --- | --- | --- | --- |
| Windows | Media Foundation H.264／HEVC／AV1，FFmpeg 軟解 | 原生建立／操作最長等待 5 秒、銷毀最長等待 1 秒；控制端程序結束最多等待連線清理 3 秒 | 有序背景佇列；取消不持鎖等待傳輸 |
| macOS | VideoToolbox H.264／HEVC／AV1 編碼與 H.264／HEVC 解碼；AV1 沿用 FFmpeg 包裝的 VideoToolbox／CPU 備援 | 共用原生資源工作者，建立／操作最長等待 5 秒、銷毀最長等待 1 秒；共用桌面連線清理上限 | 共用桌面有序輸入與重設屏障 |
| Android | MediaCodec H.264／HEVC 硬解，JPEG 作相容備援 | 原生方法移至單一背景工作者；UI 不等待解碼或關閉。watchdog 於原生操作超過 5 秒後退役該工作者，撤回影片能力 | Go 核心的有序背景佇列，合併相鄰同螢幕移動，按鍵與按鈕保留順序 |

這些時間是各次呼叫端等待／偵測的期限，不是端到端延遲保證，也不是對全部視窗、繪圖、聲音或作業系統 API 的統一時限。Android 仍是 Viewer，不增加手機被控模式或未實作的軟體影片 decoder。

## 原生資源所有權

建立、操作與銷毀由同一個原生工作者執行。逾時後只停止等待並退役，不在另一執行緒釋放仍被使用的 session，也不強制終止原生呼叫。桌面輸入採獨立副本，編碼像素可在原生操作完成後回到緩衝池，不能在逾時時提早重用；已解碼輸出不借用呼叫端的可變緩衝。

原生呼叫若稍後返回，仍由原擁有者回收；若始終不返回，資源會保留至程序退出。停滯後不在同一程序反覆建立相同後端：Windows 停用後續 Media Foundation／D3D 建立，macOS 按編碼／解碼格式退役，FFmpeg 按後端退役；Android 撤回硬體影片路徑，後續 Activity 亦不再建立該類工作者。正常的過載撤回則只影響當次連線，不改寫永久偏好或硬體分析結果。

Android TextureView 銷毀時回傳 `false`，由程式保留 SurfaceTexture，排入重設與延後釋放工作；原生 renderer 停止後才釋放 Surface／SurfaceTexture。UI 不會等待驅動，也不讓系統提前銷毀仍在使用的輸出目標。此作法依 [Android 的 SurfaceTexture 生命週期約定](https://developer.android.com/reference/android/view/TextureView.SurfaceTextureListener#onSurfaceTextureDestroyed(android.graphics.SurfaceTexture))；MediaCodec 輸出仍沿用 [Surface 模式](https://developer.android.com/reference/android/media/MediaCodec)。

## 佇列與過載

- 桌面接收解碼維持兩個槽位，包含處理中的影格；網路回呼不等待滿載的 decoder。
- Android Go 接收端將影片限制為兩張；MediaCodec 背景交接另有最多兩個槽位，包含處理中工作。這是各階段上限，不宣稱整條管線只有兩張，也不包含驅動內部緩衝。
- Android JPEG 仍保留原有最多 512 個區塊、32 MiB 上限，因為一張桌面可拆成多個相依 patch；不把區塊數當成影片幀數。影片交接滿載時丟棄失效相依鏈，要求新的 IDR 復原，不任意用新 P frame 覆寫舊參考影格。
- 各影片格式首張初始化不納入過載判定。連續三次處理至少 120 ms 且每次都有新增滿載丟幀時，撤回該格式，沿用能力協商切換其他候選，最後保留 JPEG。健康影格或沒有新的排隊壓力會中斷連續計數。
- Android 從背景輸出的快照更新畫面／輸入準備狀態；重連、換螢幕、暫停或 Surface 變更後，舊世代的輸出不更新新畫面。

Android 缺幀恢復與 Surface／連線重建分開：同一缺口只淘汰一次舊工作，保留已配置的 MediaCodec 並等待 IDR，避免硬體初始化慢於兩幀時反覆建立、銷毀而沒有輸出。Surface 模式且支援 `FEATURE_AdaptivePlayback` 的候選直接從新 IDR 接續；不支援者使用同步 `flush()`，若尚未收到第一筆輸出或格式變更，補送可能被 flush 清除的 CSD。硬體能力不明時採後者。這遵循 [MediaCodec 的串流邊界與自適應播放規則](https://developer.android.com/reference/android/media/MediaCodec#seeking--adaptive-playback-support)，並非依廠牌或型號繞過恢復。Surface 改變、換螢幕、重連與退出仍使用原生重建／延後釋放流程。

三平台的鍵鼠傳送皆不讓 UI 持鎖等待網路。相鄰移動可合併，不能跨越按鍵、按鈕、滾輪或重設屏障。取消時清除未傳送的舊輸入；若按下事件已在傳送中，工作者完成後會先釋放實際送出的按鍵／按鈕，再處理重設後的新輸入。

桌面 MCP 文字貼上的按鍵組也沿用同一工作者，保持順序並等待整組背景傳輸呼叫返回；等待與取消不持有輸入鎖，失焦或取消後不補送剩餘貼上鍵。這是傳輸交接結果，沒有增加遠端應用程式的貼上完成確認。

IP 直連立即重連時，前一連線的 Host 清理可能尚未完成。桌面與 Android 共用的直連客戶端只針對此時的 HTTP 409，在既有十秒總期限內每 200 ms 重試，並接受取消；其他 HTTP、TLS 或網路錯誤直接回報。成功後仍使用該次 TLS exporter 綁定與密碼認證，不放寬同時連線或授權規則。

## 驗證與限制

- 桌面 Go／race Smoke 模擬建立、編解碼及銷毀停滯，驗證等待期限、退役、像素／壓縮資料副本、狀態讀取與資源不提前釋放；另驗證正式雙槽交接的慢速解碼備援。
- macOS ARM64 執行實際 VideoToolbox H.264／HEVC GOP、FFmpeg H.264／HEVC／AV1 與正式影音建置標籤的 Viewer Smoke，並編譯 Client／Remote。
- Android JVM Smoke 使用真正的背景交接與 watchdog，原生引擎採可控制的阻塞替身；驗證 UI 不等待、過載判定、舊影格取消、Surface 釋放順序及停滯後不重建工作者。既有 MediaCodec 解析／候選／輸入槽位測試與前端操作回歸一併保留。
- Android Go／race Smoke 驗證影片雙槽、JPEG 多區塊保留、滿載 IDR 復原，以及阻塞傳輸時的 5,000 筆移動、重設與按鈕釋放；AAR 核對來源指紋、個人路徑與 16 KB ELF。
- Android Release lint、Debug／Release APK 編譯及 APK 原生庫／ZIP 對齊驗證通過。此為建置檢查，未另行發布新版本。

10 月 20 日以 ASUS／Android 11 補做實機驗證，包含 H.264／HEVC 1080p GOP、缺幀、背景恢復、串流中控制與關閉及立即重連；結果與限制見 [Android 實機紀錄](ANDROID-DEVICE-2026-10-20.md)。上述 Smoke 與編譯不等同全部廠牌驅動或數小時負載驗收。Windows 原始回報的實機根因仍未確定，詳見 [Windows 卡頓紀錄](WINDOWS-IDLE-INPUT.md)。本地離線更新另見 [DMG／Portable ZIP 更新與回退](LOCAL-UPDATE.md)，不改動 Android 原有的 APK 更新與系統安裝確認。

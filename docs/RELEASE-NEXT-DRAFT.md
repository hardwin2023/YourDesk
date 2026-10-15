# 待發行修正

函式級最佳化、Android APK 自動更新與四語文件已整理至 [1.26.1003 build 2028](RELEASE-1.26.1003-build-2028.md)。Android 與桌面版合併發行；Windows 持續採 Portable ZIP／Service ZIP。後續變更由此繼續記錄。

## 已實作、待發行

- Windows 桌面存取失敗後清除失效擷取資源，回到同名桌面仍重新綁定；輸入注入失敗後只處理新事件，不重播被拒絕的輸入。
- DXGI 暫時桌面／裝置切換一秒後重試；串流恢復淘汰舊世代並送完整影格。
- [UAC 修正與驗證界線](WINDOWS-UAC-2026-10-07.md)：未取得系統權限的普通 Host 仍不能操作 UAC。Windows 新增「授權系統操作」Switch，與登入前啟動分開；首次手動授權後，APP 以租約維持服務 Host，退出／工作階段切換時回收。單純 UAC 授權不改動 Windows SAS 原則，首次 Ctrl+Alt+Del 授權也不強制開機連線。

- 執行期間服務模式沿用編碼同步、配對驗證及免重複提權更新；更新保留登入前模式與 SAS 標記。共用生命週期、設定頁 Smoke／race 與 Windows 交叉編譯已驗證，UAC 安全桌面仍待實機驗收。

- Windows 新增獨立「移除系統服務」，修正服務離線時無法先停用再移除的操作死結；服務能力依實際租約握手確認，避免更新回復後誤信殘留標記。修正分段 JSON 尾端空白造成租約誤斷線，補齊取消／拒絕與舊版相容回歸 Smoke。

- Windows 正式串流補上 HEVC 硬體編碼，修正原生分析已成功卻無法使用的路徑缺口；改善小尺寸能力判定、MFT 品質／碼率控制協商及 RGBA／BGRA 輸入相容性。選單仍依正式路徑成功證據判定，保留驅動失敗原因；HD Graphics 530 實機待驗收。

- 改善鍵鼠排隊：控制端與接收端合併連續滑鼠移動，保留最新位置，避免舊位置拖慢後面的按鍵。保留拖曳、修飾鍵、滾輪、貼上與取消界線及 Windows 輸入桌面檢查；大量移動與順序 Smoke／race 通過，實機手感待確認。

- 修正右上角把服務模式當成連線狀態的誤導：改查實際 Host ID 在配對伺服器的註冊結果，區分在線、離線與未知；保留已建立的 P2P 狀態優先。服務模式及版本差異放在說明，手動 ZIP 更換文件同步釐清既有服務副本不會自動更新。使用者確認移除服務後連線恢復，尚未取得故障服務日誌。

- 「關於」新增[本地離線更新](LOCAL-UPDATE.md)，可選 Windows Portable ZIP 或 macOS DMG，允許升版、同版修復與回退。先驗證內容與架構，按「開始更新」才關閉、同步替換 APP／已安裝服務並重開；保留設定與服務模式，失敗嘗試回復。Windows 本地服務更新需本次 UAC 授權，原有線上 OTA 流程不變。原生 DMG、選檔、四語介面與隔離更新腳本 Smoke 通過；Windows 實機服務交接仍待驗收。

- [漸進卡頓與控制端關閉](WINDOWS-IDLE-INPUT.md#漸進卡頓與控制端無法關閉2026-10-15)：持續解碼過慢且滿載丟幀時，當次連線撤回該影片格式並協商備援；Media Foundation／D3D 與 FFmpeg 解碼工作者加入等待期限及安全退役。鍵鼠取消不再持鎖等待傳輸，程序結束的連線清理限制等待三秒。新增解碼耗時、滿載丟幀及過載切換記錄；壓力、順序、原生有效／損毀／GOP 與 race Smoke 通過，Windows 長時間實機根因仍待確認。

## 原始碼同步與驗證紀錄（2026-10-15）

本輪再將[串流過載與安全退出](STREAMING-RECOVERY.md)套用至 macOS／Android：macOS VideoToolbox 編解碼加入原生工作者期限；Android MediaCodec 改走有界背景交接、逾時退役與延後 Surface 釋放，Go 核心補上影片雙槽及有序輸入。macOS 實際影音 Smoke／Client／Remote 建置、Android Go／race、JVM／前端 Smoke、AAR／APK 對齊、Release lint 及 Debug／Release APK 編譯通過；未連接 Android 實機，也未建立新 Release。

本次同步包含 Windows 系統服務移除與租約相容性、HEVC 正式硬體路徑、鍵鼠有序佇列、實際 Host 連線狀態、本地離線更新與回退，以及解碼過載／原生等待／控制端結束處理。

- Go／race Smoke 已涵蓋本地更新、服務交接與相容性、輸入順序與取消、雙槽解碼過載、原生工作者逾時及資源所有權。
- macOS ARM64 執行實際 FFmpeg H.264／HEVC／AV1 解碼與 GOP 測試；Windows x64／ARM64 已完成正式影音建置標籤的編譯與封裝。Windows 驅動、UAC／SCM 交接、HD Graphics 530 及長時間連線仍待實機驗收。
- 管理介面的本地更新、取消／回退確認、實際 Host 在線狀態及服務移除已完成隔離瀏覽器 Smoke；Windows PowerShell／macOS 更新腳本在隔離環境驗證失敗回復，不操作真實系統服務。
- 已交付本機 Windows x64／ARM64 Portable ZIP 測試套件 `1.26.1015 build 1525`，核對完整 SHA-256 清單、架構、版本、四語更新重點及服務副本，並以正式本地更新流程完成上傳／驗證／取消 Smoke。

原始碼同步與上述本機測試套件不代表已發布 GitHub Release。目前公開下載仍以 [1.26.1003 build 2028](RELEASE-1.26.1003-build-2028.md) 的發行內容為準。

發布前必查：產品與版本只填入 GitHub Release 標題欄；正式內文直接由 `## 繁體中文` 開始，不重複放置版本標題，也不將本草稿的標題或提醒一併上傳。發布後讀回標題與內文核對，詳見 [Release 標題與內文檢查](RELEASE-WORKFLOW.md#release-標題與內文檢查每次建立或修改都必做)。

# 閒置斷線修正與發行驗證

日期：2026-10-23。版本：[1.26.1023 build 1131](RELEASE-1.26.1023-build-1131.md)。Android versionCode：`29878771`。

## 修正範圍

- 桌面心跳將已驗證版本與請求 ID 的拒絕回覆視為通道可往返，避免忙碌或雙端時鐘差異造成誤斷線。一般命令仍回報原本的錯誤，時效與授權檢查保留。
- 桌面共用 P2P 與 Android 不因 ICE `Disconnected` 立即拆線，沿用底層恢復期限；進入 `Failed` 或持續無回應時仍結束，沒有無限等待。
- 桌面主介面保留遠端結束、網路恢復逾時、心跳及控制事件故障原因，遠端視窗自動關閉後仍可看到。主動關閉不顯示故障提示。診斷紀錄增加 ICE／Peer 狀態轉換。

關閉「長時間未操作自動關閉」時，無輸入、靜止畫面或低流量本身不觸發閒置關閉；本次未變更使用者偏好、協定或編解碼選項。Windows、macOS 及 Linux／WinPE Host 使用桌面共用核心；Android 使用對應核心。

## 已完成的原始碼與 Smoke 驗證

修改前重現兩類誤判：心跳收到拒絕回覆仍被關閉，以及可恢復的 UDP 暫斷立即關閉。修改後的原始碼雜湊已在發行準備時與驗證紀錄逐一核對。

- 六種命令拒絕結果各模擬一小時閒置，維持連線；另透過實際 DataChannel 驗證時鐘差異、命令拒絕及存活判定。
- 桌面與 Android core 使用真實 ICE／DTLS／SCTP 連線：模擬封包暫斷後沿用同一工作階段恢復，持續失聯仍在期限內關閉。
- 桌面 `internal/p2p`、`internal/idleclose`、Android core 全部套件與斷線提示／偏好相關 Smoke 通過 Go race。
- 主介面以實際子程序事件驗證網路／心跳故障提示與正常手動關閉；語法、gofmt、JSON 翻譯及差異空白檢查通過。
- macOS arm64、Windows x64／ARM64 的 Host、Viewer、主介面含正式影音依賴完成編譯 Smoke；macOS Host／Viewer 啟動成功。Android AAR、Debug／Release APK 與 16 KB 原生對齊建置檢查通過。

本機證據：`.local-run/idle-disconnect-before.log`、`.local-run/idle-disconnect-after.log`、`.local-run/idle-disconnect-20261023-111753/verification.json` 及該目錄的各平台建置／race 日誌。這些含機器環境的記錄不提交至 Git。

## 發行核對要求

本輪正式套件從本次提交重新建置，保留 Windows x64／ARM64 Portable 與服務 ZIP、macOS arm64 DMG、Android arm64 ZIP、Linux x64／arm64 CLI ZIP、WinPE x64 實驗性 ZIP。公開附件不含 NSIS 安裝 EXE 或獨立 APK。

發布前必須完成：套件版本／架構／檔案雜湊；Windows 服務 manifest 與 DLL；正式 DMG 內 App 與映像的 Developer ID 簽章、Apple 公證、票證及 Gatekeeper；Android Release lint、相同簽章、版本碼遞增、APK v2／v3 與 16 KB 對齊；四語 Release Note；GitHub 草稿全部附件大小／SHA-256 核對。通過後才正式發布並讀回確認。

本輪發行證據存放於 `.local-run/release-1.26.1023-build-1131/`，包括 `package-verification.log`、`verified-assets.json`、`github-draft-verified.json` 與發布後的 `github-published-verified.json`；原始碼、標籤與 Android 建置紀錄須對應同一提交。

## 驗證界線

尚未取得使用者故障當下的日誌、斷線時間或雙端網路狀態，因此不能認定每次現場斷線都是這兩項問題。模擬一小時是虛擬時間；真實 P2P Smoke 使用本機封包中斷，未涵蓋跨機數小時 Wi-Fi、休眠及網路切換。

本輪不將跨平台編譯視為 Windows／WOA 驅動、UAC／SCM 或 Android 裝置實機驗收。10 月 20 日 ASUS／Android 11 的實機結果仍是[歷史驗證](ANDROID-DEVICE-2026-10-20.md)，沒有重述為本輪新套件結果。Linux／WinPE 實機、Android 16／16 KB 裝置及長時間負載仍待驗收。遠端聲音、登入前服務及 WinPE 維持實驗性。

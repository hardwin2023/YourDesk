# Windows UAC 安全桌面與恢復修正（2026-10-07）

## 回報與原因

一般 Windows Host 遇到 UAC 管理員授權畫面後，畫面不再更新，鍵鼠也無法操作。此次確認遠端沒有開啟登入前服務，因此 Host 仍以一般使用者身分執行。

UAC 的安全桌面不是一般的全螢幕視窗。[Microsoft 的 DuplicateOutput 說明](https://learn.microsoft.com/en-us/windows/win32/api/dxgi1_2/nf-dxgi1_2-idxgioutput1-duplicateoutput)指出，LOCAL_SYSTEM 才能使用這個介面擷取安全桌面；未取得對應桌面權限時不能以舊桌面繼續擷取或注入輸入。桌面與顯示模式切換亦會使 duplication 失效，需釋放及重建，見 [AcquireNextFrame](https://learn.microsoft.com/en-us/windows/win32/api/dxgi1_2/nf-dxgi1_2-idxgioutputduplication-acquirenextframe)。

「開機／登入前接受連線」與「執行期間有權操作 UAC」是兩種能力。[AnyDesk 官方文件](https://support.anydesk.com/docs/portable-vs-installed)區分未提權的 Portable、已提權 Portable 與安裝服務三種模式；已提權 Portable 可操作 UAC，且不需要開機自動啟動。[UAC 說明](https://support.anydesk.com/docs/administrative-privileges-and-elevation-uac)也提供請求提權與接受提權連線的入口。其他軟體的功能開關關閉，不能直接推論背景沒有服務或提權 helper。

## 已套用的恢復修正

- 擷取執行緒在桌面存取失敗時釋放 DXGI／GDI 資源，標記舊綁定失效；即使恢復為同名 Default 桌面，也重新綁定並建立擷取資源。持續拒絕期間不每幀反覆重建。
- 輸入執行緒在開啟桌面、綁定或注入失敗後，下個新事件重新確認並綁定桌面。被拒絕的事件直接失敗，不排隊等待授權後重播。
- 桌面切換、裝置遺失／重設、工作階段中斷與暫時拒絕存取的 DXGI 失敗，一秒後重試；不支援的後端及其他錯誤仍使用三十秒退避。
- 擷取恢復後提高影格世代，淘汰中斷前的待傳影格，重新建立編碼參考並傳送完整影格。此處不依賴有沒有設定 UI 狀態回呼。

這些修正處理中斷後恢復；不會把一般使用者的 Host 自動提升為 SYSTEM。下列獨立授權模式已接上受保護服務；實際 UAC 畫面與輸入仍須 Windows 實機驗收。

## 獨立系統授權模式

本輪重用受保護的 YourDesk Windows 系統服務，將遠端連線的啟動條件改為以下兩種：

| 登入前啟動 | APP 是否執行 | 是否接受遠端連線 |
| --- | --- | --- |
| 關閉 | 未執行 | 否 |
| 關閉 | 執行中且有有效租約 | 是，使用已授權的 SYSTEM Host |
| 開啟 | 任意 | 是，維持登入前連線模式 |

首次由使用者手動授權安裝 Program Files 中的受保護服務副本，Windows 顯示一次 UAC；不在每次 APP 啟動要求授權。設定與模式標記僅 SYSTEM／管理員可寫。APP 以核對服務身分的本機私有管線持有租約，服務核對呼叫者 PID 與目前主控台工作階段；APP 退出、崩潰、管線斷線或切換 session 時回收租約。未開啟登入前連線時，沒有租約即回收 Host。背景 helper 的安裝仍會保留，供下一次 APP 使用。

服務只提供固定模式設定與租約，不接受任意執行路徑或命令。保留既有遠端配對驗證、服務目錄 ACL、更新套件驗證及 SYSTEM 禁用遠端終端機的界線。UAC 系統原則維持原狀，Ctrl+Alt+Del 原則仍由既有明確授權流程處理。

Windows「進階設定」新增「授權系統操作」Switch。手動開啟並確認才安裝；APP 啟動、背景輪詢及重新開啟不自動詢問。若先授權 Ctrl+Alt+Del，也以執行期間模式安裝服務，不強制開啟登入前啟動。單純授權 UAC 控制會建立 YourDesk 的 SAS 停用標記，既有 Windows SAS 原則即使允許也不自動打開 YourDesk 的 Ctrl+Alt+Del；需另外使用 SAS Switch 明確授權。

已安裝新版服務時，登入前啟動透過已驗證管線變更固定模式，不再安裝或移除服務，也不再要求 UAC。關閉登入前啟動後，APP 的有效租約仍維持 Host；關閉 APP 才回收。正常操作可先關閉登入前啟動，再關閉「授權系統操作」。進階設定另提供「移除系統服務」，可直接在管理員授權後清除登入前啟動與系統操作服務；服務停止、管線失效或模式標記損壞時也可使用，不必先透過失效的管線關閉開機設定。macOS／WinPE 不顯示新增的系統操作開關，macOS 原有服務行為維持。

SCM 的背景 helper 保持自動啟動，方便後續免重新授權；「登入前啟動」關閉時，沒有 APP 租約就不建立代理、Host 或遠端監聽。受保護的 `autostart.disabled` 保存此模式，`desktop-control.v1` 保留為安裝標記，APP 不再以它判斷執行中的服務能力。模式標記錯誤、目錄連結或讀取失敗均停止服務建立 Host；舊版沒有標記時保留原開機連線模式。APP 不會把服務失敗降回另一份普通 Host。

管線先核對 SCM 服務 PID／LocalSystem／固定執行參數，服務再從 OS 管線 PID 取得呼叫者工作階段，拒絕非目前主控台的租約與模式設定，不接受 JSON 指定身分。長連線最多四份，保留其餘控制席位給設定與更新。工作階段切換會關閉舊租約；回到原工作階段時 APP 必須重新握手，延遲的舊回收不能扣除新租約。握手有期限與長度上限，額外操作會結束租約。

服務 Host 的配對密碼、狀態／斷線、編碼同步及自動更新都改以「服務已安裝」判斷，避免執行期間模式建立第二份 Host、誤用使用者密碼，或漏更新受保護副本。更新器仍只替換官方驗證的服務執行檔及影音 DLL，配對資料、模式與 SAS 標記不在替換清單內。

## 驗證界線

新模式的共用生命週期與管線 Smoke 已通過：APP／服務退出、主控台切換、相同 session 再次取得租約、並行回收、租約數量上限、握手取消、額外資料拒絕，以及模式持久化失敗時保留舊狀態。設定 API 驗證非法組合、非 Windows 授權與服務移除順序。真實設定頁的瀏覽器 Smoke 確認取消不安裝、啟動不詢問、UAC 與開機連線獨立、SAS 分開授權及已授權開關不重複提權；設定頁截圖亦檢查完成。

已通過桌面綁定、同名桌面恢復、連續一百次拒絕、輸入不重播及 DXGI 重試策略回歸測試。擷取恢復與同時切換視野的世代測試亦通過，防止中斷前的影格或舊視野在恢復後誤送。共用桌面、輸入與串流 smoke／race 測試通過，Windows x64／ARM64 的 desktop、input、hostsession 測試執行檔均完成原生 CGO 交叉編譯；另確認 x64 的 FFmpeg 發行組態可連結擷取修正。

驗證記錄位於本機 `.local-run/uac-recovery-smoke.log`、`.local-run/uac-stream-recovery-smoke.log` 及 `.local-run/uac-desktop-build-amd64.log`；交叉編譯產物位於 `.local-run/uac-cross-amd64`／`.local-run/uac-cross-arm64`。本機未指定已建置的 Remote，`TestAgentVideoHelperSmoke` 的外部程序子案例跳過；不宣稱已完成該程序或 Windows 實機測試。

Windows x64／ARM64 均已完成新版 prelogin／clientui 測試執行檔及完整 Client 建置，Client 使用發行組態的 `turbojpeg,ffmpeg,opus` 原生影音依賴；建置記錄在 `.local-run/uac-system-build-amd64.log`／`.local-run/uac-system-build-arm64.log`。

新模式的測試記錄在 `.local-run/uac-system-smoke.log`，瀏覽器驗證使用 `scripts/smoke-sas-permission.cjs`。Windows 原生標記／ACL 測試已加入 Windows 測試執行檔，交叉編譯完成不代表已在 Windows 執行。

本機為 macOS，未連接可執行 UAC 的 Windows 測試主機。測試替身與交叉編譯不等於已實機確認安全桌面的畫面與輸入。仍須在 Windows 分別驗收普通 Host、已授權服務 Host、UAC 接受／取消、反覆切換、UAC 期間斷線重連及長時間連線。

## 2026-10-13：靜態檢視問題修正

- 新增 Windows「移除系統服務」入口。即使登入前啟動仍開啟、服務停止、管線無法回覆或模式損壞，也能在使用者確認與 Windows UAC 授權後執行固定的服務移除。取消或授權拒絕保留原狀；變更期間禁止重複操作，macOS／WinPE 不提供此入口。
- 租約能力由已驗證的管線握手成功才標為可用；管線結束或新握手失敗時立即清除。回復舊版服務後，即使磁碟殘留新版能力標記，也不會假裝服務支援租約。登入前設定每次以服務實際回覆確認；新版成功保存模式，舊版不認識操作時僅在使用者明確要求關閉的情況沿用原 UAC 移除。服務離線或明確拒絕設定時，回報失敗，由使用者選擇獨立移除，不自動升權重試。
- 租約客戶端保留 JSON 解碼器的緩衝內容，與後續管線資料接續讀取。分段回覆的換行／空白不再終止租約；同次讀取或後續讀取的額外非空白操作均拒絕。EOF、取消、服務退出及主控台切換仍回收租約。

本輪新增分段回覆、緩衝內額外指令、回復不支援租約的舊服務、服務停止時清除能力、登入前設定遭拒不自動提權，以及離線／損壞服務的移除 API 驗證。設定頁 Smoke 使用實際 HTML／JS 與隔離 API，驗證移除確認、取消、授權拒絕、完成後兩個開關關閉、變更期間禁用及其他平台隱藏。驗證記錄在 `.local-run/windows-review-smoke.log`／`.local-run/windows-review-ui-smoke.log`；Windows x64／ARM64 編譯記錄在 `.local-run/windows-review-build-amd64.log`／`.local-run/windows-review-build-arm64.log`。Windows SCM 停止／刪除與 UAC 對話框仍需實機驗收。

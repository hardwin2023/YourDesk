## 繁體中文

- 改善 Windows、macOS 與 Android 的串流過載、鍵鼠排隊及控制端關閉。持續解碼過慢且丟幀時，當次連線會重新協商備援格式；原生編解碼等待與 UI 分開處理，避免累積舊影格與輸入。
- 修正 Android 硬體初始化與缺幀恢復反覆重建、無法呈現畫面的問題；保留有效解碼配置並等待新的關鍵影格。桌面與 Android 的 IP 直連遇到前一工作階段清理時，會在原連線期限內重試，改善立即重連。
- Windows 補上正式串流的 HEVC 硬體編碼路徑，改善能力判定、MFT 協商、輸入格式及桌面切換恢復。「授權系統操作」與「登入前啟動」分開，新增「移除系統服務」，並以實際 Host 註冊結果顯示連線狀態。
- 桌面版「設定 → 關於 → 本地更新」可選 Windows Portable ZIP 或 macOS DMG，支援離線升版、同版修復及回退。確認開始後自動關閉、替換 APP／已安裝服務並重開，保留設定與服務模式；失敗會嘗試回復。

套件包括 macOS Apple Silicon DMG、Windows x64／ARM64 Portable 與服務 ZIP、Linux x64／arm64 CLI Host ZIP、WinPE x64 實驗性 ZIP，以及 [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1020-build-1252/YourDesk-1.26.1020-build-1252-android-arm64.zip)。macOS App 與 DMG 均以 Developer ID 簽章、通過 Apple 公證並附加公證票證；需允許螢幕錄製與輔助使用。Linux 套件沒有桌面 Viewer。

Windows 請完整解壓縮 `-portable.zip` 至可寫入的資料夾，保留 DLL 與授權文件，再執行 `YourDesk.exe`；需要 WebView2 Runtime。只提供 ZIP，沒有 NSIS 安裝 EXE；ZIP 內 EXE 仍未具可信程式碼簽章，可能出現系統警告。`-service.zip` 僅供已授權服務更新。線上 OTA 沿用服務交接；首次安裝或舊服務遷移需管理員授權，本地套件更新受保護服務時也需本次 UAC。首次授權系統操作後，執行期間服務可處理安全桌面，無須同時開啟登入前啟動；未授權的普通 Host 仍無此權限。本版不保證所有驅動上的 UAC 操作已通過實機驗收。舊版若需手動替換檔案，先使用「移除系統服務」再退出 APP；僅關閉登入前啟動或替換 ZIP，不會移除或更新既有服務副本。

Android 適用 Android 8.0 以上的 arm64 手機 Viewer。解壓縮 ZIP，再安裝其中正式簽章 APK；沿用相同簽章與 applicationId，可覆蓋更新而不需卸載。`1.26.1003 build 2028` 以前的版本須先手動更新一次；有更新器的版本可在首頁前景閒置時自動檢查及下載，連線或進入背景會停止。安裝仍須使用者及 Android 確認，必要時允許 YourDesk 安裝來源。手機被控、剪貼簿同步、獨立檔案傳輸及 AV1 不在本版 Android 範圍。

驗證涵蓋相關 Go／race、輸入順序與過載／退出 Smoke、macOS 原生影音、隔離本地更新及服務交接腳本、各平台發行建置與套件檢查。ASUS_I01WD／Android 11 的 42 項實機測試全數通過；H.264／HEVC 1080p 串流各三分鐘，平均實際呈現 39.43／47.15 FPS，最慢 UI 心跳 18 ms、控制屏障往返 128 ms，串流中關閉 140 ms，缺幀、背景返回與立即重連成功。另通過 25 項 JUnit、10,630 個解析／MediaCodec 替身斷言、Release lint（0 error／fatal、31 warning）、APK v2／v3 簽章延續性、16 KB ELF／ZIP 對齊及 SHA-256 檢查。

上述手機測試為單一裝置與合成 LAN 來源，不能證明 Windows 原始卡頓根因或整體操作延遲；Windows／WOA 驅動、HD Graphics 530、UAC／SCM 與更新實機交接、Linux／WinPE 實機、Android 16／16 KB 裝置及數小時負載仍待驗收。遠端聲音、登入前服務與 WinPE 維持實驗性。

技術文件：[串流恢復](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md)、[Android 實機紀錄](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-DEVICE-2026-10-20.md)、[本地更新](https://github.com/VaderChen/YourDesk/blob/main/docs/LOCAL-UPDATE.md)、[修正與驗證範圍](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-20.md)。

## English

- Improve streaming overload, input queues and viewer shutdown on Windows, macOS and Android. Sustained slow decoding with dropped frames renegotiates fallback formats for the current connection; native codec waits run separately from the UI to limit stale frames and input.
- Fix repeated Android decoder recreation during hardware initialization and frame-loss recovery. Retain a valid configuration and wait for a new keyframe. Desktop and Android direct-IP connections retry within the existing deadline when the previous session is still cleaning up, improving immediate reconnection.
- Enable the Windows production HEVC hardware encoding path and improve capability detection, MFT negotiation, input formats and desktop-switch recovery. Separate system-control authorization from pre-login startup, add Remove system service, and display connectivity using the actual Host registration.
- Desktop Settings → About → Local update accepts Windows Portable ZIPs or macOS DMGs for offline upgrades, same-version repair and rollback. Confirmation closes the app, replaces it and any installed service, then restarts while preserving settings and service modes; failures attempt restoration.

Packages: macOS Apple Silicon DMG; Windows x64/ARM64 Portable and service ZIPs; Linux x64/arm64 CLI Host ZIPs; experimental WinPE x64 ZIP; and [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1020-build-1252/YourDesk-1.26.1020-build-1252-android-arm64.zip). Both the macOS app and DMG are Developer ID signed, Apple-notarized and stapled. Allow Screen Recording and Accessibility permissions. Linux packages do not include a desktop viewer.

On Windows, extract the entire `-portable.zip` into a writable folder, retain the DLLs and licenses, and run `YourDesk.exe`. WebView2 Runtime is required. Only ZIPs are published, without an NSIS installer; the EXEs still lack trusted code signing and may trigger system warnings. `-service.zip` is for authorized service updates only. Online OTA retains service handoff. Initial installation or legacy migration needs administrator approval; updating a protected service from a local package also needs UAC approval for that update. Once system control is authorized, a runtime service can handle the secure desktop without enabling pre-login startup; an unprivileged Host cannot. This does not establish successful UAC operation on every driver. Before manually replacing an older installation, remove the system service and quit the app. Disabling pre-login startup or replacing ZIP files alone does not remove or update an existing service copy.

Android is a phone Viewer for arm64 devices running Android 8.0 or later. Extract the ZIP and install the officially signed APK. The signing identity and applicationId are unchanged, allowing an in-place update without uninstalling. Versions before `1.26.1003 build 2028` need one manual upgrade. Versions with the updater check and download while the home screen is idle in the foreground; connecting or backgrounding stops this work. Installation still requires user and Android confirmation; allow YourDesk as an installation source if requested. Phone hosting, clipboard sync, dedicated file transfer and AV1 are not included on Android.

Validation includes relevant Go/race checks, input-order and overload/shutdown smoke tests, native macOS media, isolated local-update/service-handoff scripts, release builds and package checks. All 42 device tests passed on ASUS_I01WD/Android 11. Three minutes each of 1080p H.264 and HEVC averaged 39.43/47.15 actually presented FPS, with a maximum 18 ms UI heartbeat and 128 ms control-barrier round trip. Closing during streaming took 140 ms; frame-loss recovery, background return and immediate reconnection passed. Also passed: 25 JUnit tests, 10,630 parser/MediaCodec-fake assertions, Release lint (0 errors/fatals, 31 warnings), APK v2/v3 signing continuity, 16 KB ELF/ZIP alignment and SHA-256 checks.

The phone results use one device and a synthetic LAN source; they do not establish the original Windows freeze cause or end-to-end interaction latency. Windows/WOA drivers, HD Graphics 530, UAC/SCM and update handoff on devices, physical Linux/WinPE systems, Android 16/16 KB devices and multi-hour workloads still need validation. Remote audio, pre-login services and WinPE remain experimental.

Technical records: [streaming recovery](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md), [Android device results](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-DEVICE-2026-10-20.md), [local updates](https://github.com/VaderChen/YourDesk/blob/main/docs/LOCAL-UPDATE.md), and [fixes and validation scope](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-20.md).

## 日本語

- Windows・macOS・Android の映像過負荷、入力待ち、Viewer 終了処理を改善しました。復号の遅延とフレーム破棄が続く場合、現在の接続で代替形式を再交渉します。ネイティブ処理の待機を UI から分離し、古い映像や入力の蓄積を抑えます。
- Android のハードウェア初期化・フレーム欠落時に復号器の再作成を繰り返す問題を修正しました。有効な設定を保持し、新しいキーフレームを待ちます。デスクトップと Android の IP 直結では、前の接続を終了処理中の場合、既存の期限内で再試行し、即時再接続を改善します。
- Windows の正式な HEVC ハードウェア符号化経路を追加し、能力判定、MFT 交渉、入力形式、デスクトップ切り替え後の復帰を改善しました。システム操作の許可をログイン前起動から分離し、サービス削除を追加しました。接続状態は実際の Host 登録結果で表示します。
- デスクトップの設定 → バージョン情報 → ローカル更新で、Windows Portable ZIP／macOS DMG によるオフライン更新、同版修復、旧版への復帰ができます。確認後に終了し、APP と既存サービスを置き換えて再起動します。設定とサービスモードを保持し、失敗時は復元を試みます。

配布物：macOS Apple Silicon DMG、Windows x64／ARM64 Portable・サービス ZIP、Linux x64／arm64 CLI Host ZIP、実験的な WinPE x64 ZIP、[Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1020-build-1252/YourDesk-1.26.1020-build-1252-android-arm64.zip)。macOS APP と DMG は Developer ID 署名・Apple 公証済みで、公証チケットも添付済みです。画面収録とアクセシビリティの許可が必要です。Linux はデスクトップ Viewer を含みません。

Windows の `-portable.zip` を書き込み可能なフォルダーに全て展開し、DLL とライセンスを保持して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。NSIS インストーラーは提供せず、ZIP 内 EXE は信頼されたコード署名が未対応のため警告が出る場合があります。`-service.zip` は承認済みサービスの更新専用です。オンライン OTA はサービスの引き継ぎを維持します。初回導入・旧サービス移行には管理者の許可が必要で、ローカルパッケージから保護サービスを更新する際にも今回の UAC 承認が必要です。システム操作を許可すると実行中サービスが安全なデスクトップを扱え、ログイン前起動の有効化は必須ではありません。権限のない Host は対応できず、全ドライバーでの UAC 操作を実機確認済みとはしていません。旧版を手動置換する前はサービスを削除して APP を終了してください。ログイン前起動の無効化や ZIP 置換だけでは既存サービスの削除・更新は行われません。

Android 8.0 以降の arm64 スマートフォン向け Viewer です。ZIP を展開し、正式署名 APK をインストールしてください。同じ署名と applicationId を使用するため、アンインストールせず更新できます。`1.26.1003 build 2028` より前の版は一度手動更新が必要です。更新機能のある版はホーム画面が前面で待機中に検出・ダウンロードし、接続やバックグラウンド移行時に停止します。インストールにはユーザーと Android の確認が必要で、要求された場合は YourDesk からのインストールを許可してください。スマートフォンの被制御、クリップボード同期、専用ファイル転送、AV1 は Android 版の対象外です。

関連 Go／race、入力順序・過負荷・終了 Smoke、macOS ネイティブ映像処理、隔離環境のローカル更新・サービス引き継ぎスクリプト、正式ビルド・パッケージ検査を実施しました。ASUS_I01WD／Android 11 の実機 42 テストは全て成功しました。1080p H.264／HEVC を各 3 分間再生し、実表示平均 39.43／47.15 FPS、UI 応答確認の最大 18 ms、入力バリア往復の最大 128 ms、配信中の終了 140 ms を記録しました。欠落・バックグラウンド復帰・即時再接続も成功しました。JUnit 25 項目、解析／MediaCodec 代替 API の 10,630 検証、Release lint（error／fatal 0、warning 31）、APK v2／v3 署名継続性、16 KB ELF／ZIP 配置、SHA-256 検査も通過しました。

スマートフォンの結果は 1 台と合成 LAN 映像によるもので、元の Windows 停止原因や操作全体の遅延を証明しません。Windows／WOA ドライバー、HD Graphics 530、UAC／SCM・更新引き継ぎ、Linux／WinPE 実機、Android 16／16 KB 端末、数時間の負荷は引き続き実機確認が必要です。リモート音声、ログイン前サービス、WinPE は実験的機能です。

技術記録：[映像復帰](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md)、[Android 実機結果](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-DEVICE-2026-10-20.md)、[ローカル更新](https://github.com/VaderChen/YourDesk/blob/main/docs/LOCAL-UPDATE.md)、[修正と検証範囲](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-20.md)。

## 한국어

- Windows·macOS·Android의 스트리밍 과부하, 입력 대기와 Viewer 종료 처리를 개선했습니다. 느린 디코딩과 프레임 폐기가 지속되면 현재 연결에서 대체 형식을 다시 협상합니다. 네이티브 코덱 대기를 UI와 분리하여 오래된 프레임과 입력이 쌓이는 것을 줄입니다.
- Android 하드웨어 초기화 및 프레임 손실 복구 중 디코더가 반복 생성되던 문제를 수정했습니다. 유효한 설정을 유지하고 새 키프레임을 기다립니다. 데스크톱·Android IP 직접 연결은 이전 세션이 정리 중이면 기존 제한 시간 내에서 재시도하여 즉시 재연결을 개선합니다.
- Windows 정식 스트리밍에 HEVC 하드웨어 인코딩 경로를 추가하고 기능 감지, MFT 협상, 입력 형식 및 데스크톱 전환 후 복구를 개선했습니다. 시스템 제어 권한과 로그인 전 시작을 분리하고 서비스 제거를 추가했습니다. 연결 상태는 실제 Host 등록 결과로 표시합니다.
- 데스크톱 설정 → 정보 → 로컬 업데이트에서 Windows Portable ZIP 또는 macOS DMG로 오프라인 업그레이드, 동일 버전 복구 및 이전 버전 복원이 가능합니다. 확인 후 앱을 종료하고 앱·설치된 서비스를 교체한 뒤 다시 시작합니다. 설정과 서비스 모드를 유지하며 실패하면 복원을 시도합니다.

배포물: macOS Apple Silicon DMG, Windows x64/ARM64 Portable·서비스 ZIP, Linux x64/arm64 CLI Host ZIP, 실험용 WinPE x64 ZIP, [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1020-build-1252/YourDesk-1.26.1020-build-1252-android-arm64.zip). macOS 앱과 DMG 모두 Developer ID 서명, Apple 공증 및 공증 티켓 첨부를 완료했습니다. 화면 기록과 손쉬운 사용 권한이 필요합니다. Linux 패키지는 데스크톱 Viewer를 포함하지 않습니다.

Windows에서는 `-portable.zip` 전체를 쓰기 가능한 폴더에 풀고 DLL·라이선스를 유지한 채 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. NSIS 설치 EXE 없이 ZIP만 제공하며, 내부 EXE에는 신뢰할 수 있는 코드 서명이 없어 시스템 경고가 표시될 수 있습니다. `-service.zip`은 승인된 서비스 업데이트 전용입니다. 온라인 OTA는 서비스 인계를 유지합니다. 최초 설치나 구형 서비스 이전에는 관리자 승인이 필요하고, 로컬 패키지로 보호된 서비스를 업데이트할 때도 해당 업데이트의 UAC 승인이 필요합니다. 시스템 제어를 승인하면 로그인 전 시작을 켜지 않아도 실행 중 서비스가 보안 데스크톱을 처리할 수 있습니다. 권한 없는 Host는 처리할 수 없으며, 모든 드라이버의 UAC 동작을 실기기 검증한 것은 아닙니다. 이전 설치를 수동 교체할 때는 시스템 서비스를 제거한 뒤 앱을 종료하세요. 로그인 전 시작을 끄거나 ZIP만 교체해도 기존 서비스 사본은 제거·업데이트되지 않습니다.

Android 8.0 이상 arm64 휴대폰용 Viewer입니다. ZIP을 풀고 정식 서명 APK를 설치하세요. 기존 서명과 applicationId를 유지하므로 앱 삭제 없이 업데이트할 수 있습니다. `1.26.1003 build 2028` 이전 버전은 한 번 수동 업데이트해야 합니다. 업데이트 기능이 있는 버전은 홈 화면이 전경에서 대기 중일 때 감지·다운로드하며 연결하거나 백그라운드로 이동하면 중단합니다. 설치는 사용자와 Android 확인이 필요하며, 요청 시 YourDesk를 설치 출처로 허용하세요. 휴대폰 원격 제어, 클립보드 동기화, 전용 파일 전송과 AV1은 Android 버전에 포함되지 않습니다.

관련 Go/race, 입력 순서·과부하·종료 Smoke, macOS 네이티브 미디어, 격리된 로컬 업데이트·서비스 인계 스크립트, 정식 빌드·패키지 검사를 수행했습니다. ASUS_I01WD/Android 11의 실기기 테스트 42개가 모두 통과했습니다. 1080p H.264/HEVC 각각 3분 동안 실제 표시 평균 39.43/47.15 FPS, 최대 UI 응답 확인 18 ms, 제어 배리어 왕복 128 ms, 스트리밍 중 종료 140 ms를 기록했습니다. 프레임 손실, 백그라운드 복귀 및 즉시 재연결도 통과했습니다. JUnit 25개, 파서/MediaCodec 대체 API 검증 10,630개, Release lint(error/fatal 0, warning 31), APK v2/v3 서명 연속성, 16 KB ELF/ZIP 정렬과 SHA-256 검사도 통과했습니다.

휴대폰 결과는 단일 기기와 합성 LAN 영상에 한정되며, 원래 Windows 멈춤의 원인이나 전체 조작 지연을 입증하지 않습니다. Windows/WOA 드라이버, HD Graphics 530, UAC/SCM·업데이트 인계, Linux/WinPE 실기기, Android 16/16 KB 기기 및 수시간 부하는 추가 검증이 필요합니다. 원격 오디오, 로그인 전 서비스와 WinPE는 실험 기능입니다.

기술 기록: [스트리밍 복구](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md), [Android 실기기 결과](https://github.com/VaderChen/YourDesk/blob/main/docs/ANDROID-DEVICE-2026-10-20.md), [로컬 업데이트](https://github.com/VaderChen/YourDesk/blob/main/docs/LOCAL-UPDATE.md), [수정 및 검증 범위](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-20.md).

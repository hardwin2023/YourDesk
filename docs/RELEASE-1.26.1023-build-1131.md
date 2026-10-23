## 繁體中文

- 修正閒置時誤斷線：Windows／macOS 心跳收到忙碌或時效拒絕回覆時，仍視為通道存活，避免誤判失聯。關閉「長時間未操作自動關閉」時，不會僅因無輸入或靜止畫面結束連線。
- Windows、macOS 與 Android 保留短暫網路中斷的恢復時間，避免遇到可恢復狀態立即拆線；持續失聯仍會結束。
- 桌面自動關閉遠端視窗後，主介面保留斷線原因；主動關閉不顯示故障提示。

套件包括 macOS Apple Silicon DMG、Windows x64／ARM64 Portable 與服務 ZIP、Linux x64／arm64 CLI Host ZIP、WinPE x64 實驗性 ZIP，以及 [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1023-build-1131/YourDesk-1.26.1023-build-1131-android-arm64.zip)。macOS App 與 DMG 均以 Developer ID 簽章、通過 Apple 公證並附加公證票證；需允許螢幕錄製與輔助使用。Linux 套件沒有桌面 Viewer。

Windows 請完整解壓縮 `-portable.zip` 至可寫入的資料夾，保留 DLL 與授權文件，再執行 `YourDesk.exe`；需要 WebView2 Runtime。只提供 ZIP，沒有 NSIS 安裝 EXE；ZIP 內 EXE 仍未具可信程式碼簽章，可能出現系統警告。`-service.zip` 僅供已授權服務更新。線上 OTA 沿用服務交接；首次安裝或舊服務遷移需管理員授權，本地套件更新受保護服務時也需本次 UAC。首次授權系統操作後，執行期間服務可處理安全桌面，無須同時開啟登入前啟動；未授權的普通 Host 仍無此權限。本版不保證所有驅動上的 UAC 操作已通過實機驗收。舊版若需手動替換檔案，先使用「移除系統服務」再退出 APP；僅關閉登入前啟動或替換 ZIP，不會移除或更新既有服務副本。

Android 適用 Android 8.0 以上的 arm64 手機 Viewer。解壓縮 ZIP，再安裝其中正式簽章 APK；沿用相同簽章與 applicationId，可覆蓋更新而不需卸載。`1.26.1003 build 2028` 以前的版本須先手動更新一次；有更新器的版本可在首頁前景閒置時自動檢查及下載，連線或進入背景會停止。安裝仍須使用者及 Android 確認，必要時允許 YourDesk 安裝來源。手機被控、剪貼簿同步、獨立檔案傳輸及 AV1 不在本版 Android 範圍。

本輪通過六種心跳拒絕各一小時虛擬閒置、真實 DataChannel 往返、桌面／Android core 的 P2P 暫斷恢復與持續失聯、子程序斷線提示及 Go race。正式套件檢查涵蓋平台版本／架構、macOS 簽章／公證、Android Release lint、簽章延續性、16 KB ELF／ZIP 對齊及 SHA-256。

尚未取得使用者故障當下日誌，不能認定所有現場斷線都是相同原因。一小時閒置為虛擬時間，P2P 暫斷為本機封包測試；本輪未重做跨機數小時 Wi-Fi、休眠、網路切換或 Windows／Android 實機驗收。先前 10 月 20 日手機結果不代表本輪新套件。Windows／WOA 驅動、UAC／SCM、Linux／WinPE 實機與 Android 16／16 KB 裝置仍待驗收。遠端聲音、登入前服務及 WinPE 維持實驗性。

技術文件：[串流恢復](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md)、[本輪驗證範圍](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-23.md)。

## English

- Fix false idle disconnects: Windows/macOS heartbeats treat a valid busy or expiry rejection as a live round trip. With idle auto-close disabled, lack of input or a static screen alone does not end the session.
- Preserve recovery time for temporary network interruptions on Windows, macOS and Android instead of immediately closing a recoverable connection. Persistently unresponsive sessions still end.
- Keep the disconnect reason in the desktop main window after the viewer closes automatically. Manual closure does not show a failure notice.

Packages: macOS Apple Silicon DMG; Windows x64/ARM64 Portable and service ZIPs; Linux x64/arm64 CLI Host ZIPs; experimental WinPE x64 ZIP; and [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1023-build-1131/YourDesk-1.26.1023-build-1131-android-arm64.zip). Both the macOS app and DMG are Developer ID signed, Apple-notarized and stapled. Allow Screen Recording and Accessibility permissions. Linux packages do not include a desktop viewer.

On Windows, extract the entire `-portable.zip` into a writable folder, retain the DLLs and licenses, and run `YourDesk.exe`. WebView2 Runtime is required. Only ZIPs are published, without an NSIS installer; the EXEs still lack trusted code signing and may trigger system warnings. `-service.zip` is for authorized service updates only. Online OTA retains service handoff. Initial installation or legacy migration needs administrator approval; updating a protected service from a local package also needs UAC approval for that update. Once system control is authorized, a runtime service can handle the secure desktop without enabling pre-login startup; an unprivileged Host cannot. This does not establish successful UAC operation on every driver. Before manually replacing an older installation, remove the system service and quit the app. Disabling pre-login startup or replacing ZIP files alone does not remove or update an existing service copy.

Android is a phone Viewer for arm64 devices running Android 8.0 or later. Extract the ZIP and install the officially signed APK. The signing identity and applicationId are unchanged, allowing an in-place update without uninstalling. Versions before `1.26.1003 build 2028` need one manual upgrade. Versions with the updater check and download while the home screen is idle in the foreground; connecting or backgrounding stops this work. Installation still requires user and Android confirmation; allow YourDesk as an installation source if requested. Phone hosting, clipboard sync, dedicated file transfer and AV1 are not included on Android.

This round passed one virtual hour of idle time for each of six heartbeat rejections, actual DataChannel round trips, desktop/Android-core P2P recovery from packet loss and persistent-failure checks, subprocess disconnect notices and Go race checks. Release checks cover platform versions/architectures, macOS signing/notarization, Android Release lint, signing continuity, 16 KB ELF/ZIP alignment and SHA-256.

Incident logs are unavailable, so these fixes do not establish the cause of every reported disconnect. The hour-long idle tests use virtual time, and P2P interruptions use local packet tests. This round does not repeat multi-hour cross-device Wi-Fi, sleep, network-switching or Windows/Android device acceptance. October 20 phone results do not validate these new packages. Windows/WOA drivers, UAC/SCM, physical Linux/WinPE systems and Android 16/16 KB devices still need validation. Remote audio, pre-login services and WinPE remain experimental.

Technical records: [streaming recovery](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md), [validation scope](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-23.md).

## 日本語

- アイドル時の誤切断を修正しました。Windows／macOS はビジー・期限拒否の有効な応答も通信成立と判断します。アイドル自動終了が無効なら、無操作や静止画面だけでは接続を終了しません。
- Windows・macOS・Android で一時的なネットワーク切断の復帰時間を確保し、回復可能な状態で即時終了しないようにしました。応答が戻らない接続は終了します。
- Viewer が自動終了した後も、デスクトップのメイン画面に切断理由を残します。手動終了では障害通知を表示しません。

配布物：macOS Apple Silicon DMG、Windows x64／ARM64 Portable・サービス ZIP、Linux x64／arm64 CLI Host ZIP、実験的な WinPE x64 ZIP、[Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1023-build-1131/YourDesk-1.26.1023-build-1131-android-arm64.zip)。macOS APP と DMG は Developer ID 署名・Apple 公証済みで、公証チケットも添付済みです。画面収録とアクセシビリティの許可が必要です。Linux はデスクトップ Viewer を含みません。

Windows の `-portable.zip` を書き込み可能なフォルダーに全て展開し、DLL とライセンスを保持して `YourDesk.exe` を実行してください。WebView2 Runtime が必要です。NSIS インストーラーは提供せず、ZIP 内 EXE は信頼されたコード署名が未対応のため警告が出る場合があります。`-service.zip` は承認済みサービスの更新専用です。オンライン OTA はサービスの引き継ぎを維持します。初回導入・旧サービス移行には管理者の許可が必要で、ローカルパッケージから保護サービスを更新する際にも今回の UAC 承認が必要です。システム操作を許可すると実行中サービスが安全なデスクトップを扱え、ログイン前起動の有効化は必須ではありません。権限のない Host は対応できず、全ドライバーでの UAC 操作を実機確認済みとはしていません。旧版を手動置換する前はサービスを削除して APP を終了してください。ログイン前起動の無効化や ZIP 置換だけでは既存サービスの削除・更新は行われません。

Android 8.0 以降の arm64 スマートフォン向け Viewer です。ZIP を展開し、正式署名 APK をインストールしてください。同じ署名と applicationId を使用するため、アンインストールせず更新できます。`1.26.1003 build 2028` より前の版は一度手動更新が必要です。更新機能のある版はホーム画面が前面で待機中に検出・ダウンロードし、接続やバックグラウンド移行時に停止します。インストールにはユーザーと Android の確認が必要で、要求された場合は YourDesk からのインストールを許可してください。スマートフォンの被制御、クリップボード同期、専用ファイル転送、AV1 は Android 版の対象外です。

今回、6 種類のハートビート拒否ごとの仮想 1 時間の待機、実際の DataChannel 往復、デスクトップ／Android core の P2P パケット欠落からの復帰・継続障害、子プロセス切断通知、Go race 検査を通過しました。配布検査には各プラットフォームの版・構成、macOS 署名・公証、Android Release lint、署名継続性、16 KB ELF／ZIP 配置、SHA-256 を含みます。

障害発生時のログがないため、全ての切断原因が同じとは断定できません。1 時間の待機は仮想時間、P2P 中断はローカルのパケット試験です。今回は複数端末での数時間の Wi-Fi、スリープ、ネットワーク切り替え、Windows／Android 実機受け入れ試験を再実施していません。10 月 20 日のスマートフォン結果は今回の新パッケージの検証ではありません。Windows／WOA ドライバー、UAC／SCM、Linux／WinPE 実機、Android 16／16 KB 端末は引き続き確認が必要です。リモート音声、ログイン前サービス、WinPE は実験的機能です。

技術記録：[映像復帰](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md)、[検証範囲](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-23.md)。

## 한국어

- 유휴 상태의 잘못된 연결 종료를 수정했습니다. Windows/macOS는 바쁨 또는 기한 거부에 대한 유효한 응답도 통신 성공으로 판단합니다. 유휴 자동 종료가 꺼져 있으면 입력이 없거나 화면이 정지해 있다는 이유만으로 연결을 끝내지 않습니다.
- Windows·macOS·Android에서 일시적인 네트워크 단절 후 복구할 시간을 유지합니다. 복구 가능한 상태에서 즉시 종료하지 않으며, 응답이 지속해서 없으면 종료합니다.
- Viewer가 자동으로 닫힌 뒤에도 데스크톱 메인 화면에 연결 종료 이유를 표시합니다. 수동 종료에는 오류 알림을 표시하지 않습니다.

배포물: macOS Apple Silicon DMG, Windows x64/ARM64 Portable·서비스 ZIP, Linux x64/arm64 CLI Host ZIP, 실험용 WinPE x64 ZIP, [Android arm64 ZIP](https://github.com/VaderChen/YourDesk/releases/download/1.26.1023-build-1131/YourDesk-1.26.1023-build-1131-android-arm64.zip). macOS 앱과 DMG 모두 Developer ID 서명, Apple 공증 및 공증 티켓 첨부를 완료했습니다. 화면 기록과 손쉬운 사용 권한이 필요합니다. Linux 패키지는 데스크톱 Viewer를 포함하지 않습니다.

Windows에서는 `-portable.zip` 전체를 쓰기 가능한 폴더에 풀고 DLL·라이선스를 유지한 채 `YourDesk.exe`를 실행하세요. WebView2 Runtime이 필요합니다. NSIS 설치 EXE 없이 ZIP만 제공하며, 내부 EXE에는 신뢰할 수 있는 코드 서명이 없어 시스템 경고가 표시될 수 있습니다. `-service.zip`은 승인된 서비스 업데이트 전용입니다. 온라인 OTA는 서비스 인계를 유지합니다. 최초 설치나 구형 서비스 이전에는 관리자 승인이 필요하고, 로컬 패키지로 보호된 서비스를 업데이트할 때도 해당 업데이트의 UAC 승인이 필요합니다. 시스템 제어를 승인하면 로그인 전 시작을 켜지 않아도 실행 중 서비스가 보안 데스크톱을 처리할 수 있습니다. 권한 없는 Host는 처리할 수 없으며, 모든 드라이버의 UAC 동작을 실기기 검증한 것은 아닙니다. 이전 설치를 수동 교체할 때는 시스템 서비스를 제거한 뒤 앱을 종료하세요. 로그인 전 시작을 끄거나 ZIP만 교체해도 기존 서비스 사본은 제거·업데이트되지 않습니다.

Android 8.0 이상 arm64 휴대폰용 Viewer입니다. ZIP을 풀고 정식 서명 APK를 설치하세요. 기존 서명과 applicationId를 유지하므로 앱 삭제 없이 업데이트할 수 있습니다. `1.26.1003 build 2028` 이전 버전은 한 번 수동 업데이트해야 합니다. 업데이트 기능이 있는 버전은 홈 화면이 전경에서 대기 중일 때 감지·다운로드하며 연결하거나 백그라운드로 이동하면 중단합니다. 설치는 사용자와 Android 확인이 필요하며, 요청 시 YourDesk를 설치 출처로 허용하세요. 휴대폰 원격 제어, 클립보드 동기화, 전용 파일 전송과 AV1은 Android 버전에 포함되지 않습니다.

이번 검증은 6가지 하트비트 거부별 가상 1시간 유휴 테스트, 실제 DataChannel 왕복, 데스크톱/Android core의 P2P 패킷 손실 복구 및 지속 장애, 하위 프로세스 종료 알림과 Go race 검사를 통과했습니다. 배포 검사에는 플랫폼 버전/아키텍처, macOS 서명/공증, Android Release lint, 서명 연속성, 16 KB ELF/ZIP 정렬과 SHA-256이 포함됩니다.

장애 당시 로그가 없어 모든 연결 종료의 원인이 같다고 단정할 수 없습니다. 1시간 유휴 테스트는 가상 시간을 사용하고 P2P 단절은 로컬 패킷 시험입니다. 이번에는 여러 기기 간 수시간 Wi-Fi, 절전, 네트워크 전환이나 Windows/Android 실기기 인수 검증을 반복하지 않았습니다. 10월 20일 휴대폰 결과는 이번 새 패키지를 검증한 결과가 아닙니다. Windows/WOA 드라이버, UAC/SCM, Linux/WinPE 실기기와 Android 16/16 KB 기기는 추가 검증이 필요합니다. 원격 오디오, 로그인 전 서비스와 WinPE는 실험 기능입니다.

기술 기록: [스트리밍 복구](https://github.com/VaderChen/YourDesk/blob/main/docs/STREAMING-RECOVERY.md), [검증 범위](https://github.com/VaderChen/YourDesk/blob/main/docs/RELEASE-VALIDATION-2026-10-23.md).

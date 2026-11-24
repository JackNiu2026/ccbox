Unicode true
RequestExecutionLevel user
; Per-file decompression avoids a full payload copy in the system TEMP drive.
SetCompressor lzma
SetCompressorDictSize 32
ManifestDPIAware true
ManifestSupportedOS all

!include MUI2.nsh
!define MUI_ICON "Cage.ico"
!define MUI_UNICON "Cage.ico"
!include LogicLib.nsh
!include FileFunc.nsh
!include x64.nsh
!include WinVer.nsh
!include nsDialogs.nsh

!ifndef APP_DIR
  !error "APP_DIR is required"
!endif
!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef VERSION_NUMERIC
  !error "VERSION_NUMERIC is required"
!endif
!ifndef OUTPUT_FILE
  !error "OUTPUT_FILE is required"
!endif

!define APP_ID "org.ccbox.desktop"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\ccBox"
!define WEBVIEW2_APP_ID "{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
Name "CCBox ${VERSION}"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\CCBox"
InstallDirRegKey HKCU "${UNINSTALL_KEY}" "InstallLocation"
VIProductVersion "${VERSION_NUMERIC}"
VIAddVersionKey /LANG=1033 "ProductName" "CCBox"
VIAddVersionKey /LANG=1033 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=1033 "FileDescription" "CCBox Windows Installer"
VIAddVersionKey /LANG=1033 "CompanyName" "CCBox"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Cage contributors"

!ifdef SIGN_SCRIPT
  !uninstfinalize '"${SIGN_PWSH}" -NoProfile -File "${SIGN_SCRIPT}" -Path "%1"' = 0
!endif

!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\Cage.exe"
!define MUI_FINISHPAGE_RUN_NOTCHECKED
!define MUI_FINISHPAGE_TEXT "$(CageFinishText)"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${APP_DIR}/LICENSE.txt"
!define MUI_DIRECTORYPAGE_TEXT_TOP "$(CageDirectoryHelp)"
!insertmacro MUI_PAGE_DIRECTORY
Page custom ShortcutOptionsPage ShortcutOptionsLeave
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "TradChinese"

LangString CageDirectoryHelp ${LANG_ENGLISH} "To upgrade, keep the current CCBox installation folder. For a first installation, choose a dedicated application folder."
LangString CageDirectoryHelp ${LANG_SIMPCHINESE} "升级时请保留原 CCBox 安装目录。首次安装可选择独立的程序目录。"
LangString CageDirectoryHelp ${LANG_TRADCHINESE} "升級時請保留原 CCBox 安裝目錄。首次安裝可選擇專用的應用程式資料夾。"
LangString CageUpgradeDirectoryError ${LANG_ENGLISH} "Upgrade CCBox in the original installation folder: $0"
LangString CageUpgradeDirectoryError ${LANG_SIMPCHINESE} "请在原安装目录升级 CCBox：$0"
LangString CageUpgradeDirectoryError ${LANG_TRADCHINESE} "請在原安裝目錄升級 CCBox：$0"
LangString CageShortcutsTitle ${LANG_ENGLISH} "Shortcuts"
LangString CageShortcutsTitle ${LANG_SIMPCHINESE} "快捷方式"
LangString CageShortcutsTitle ${LANG_TRADCHINESE} "捷徑"
LangString CageShortcutsHelp ${LANG_ENGLISH} "Choose where to add CCBox shortcuts for your Windows account."
LangString CageShortcutsHelp ${LANG_SIMPCHINESE} "选择为当前 Windows 用户添加的 CCBox 快捷方式。"
LangString CageShortcutsHelp ${LANG_TRADCHINESE} "選擇要為目前 Windows 使用者新增的 CCBox 捷徑。"
LangString CageDesktopShortcut ${LANG_ENGLISH} "Add a desktop shortcut"
LangString CageDesktopShortcut ${LANG_SIMPCHINESE} "添加桌面快捷方式"
LangString CageDesktopShortcut ${LANG_TRADCHINESE} "新增桌面捷徑"
LangString CageStartMenuShortcuts ${LANG_ENGLISH} "Add to the Start menu"
LangString CageStartMenuShortcuts ${LANG_SIMPCHINESE} "添加到开始菜单"
LangString CageStartMenuShortcuts ${LANG_TRADCHINESE} "新增至開始功能表"

LangString CageFinishText ${LANG_ENGLISH} "CCBox is installed.$\r$\n$\r$\nOpen CCBox with the same Windows account to use your existing environment. If first-time setup appears unexpectedly, stop and check your original storage location."
LangString CageFinishText ${LANG_SIMPCHINESE} "CCBox 已安装。$\r$\n$\r$\n请使用同一个 Windows 账户打开 CCBox，以继续使用原有环境。如果意外出现首次设置，请先核对原存储位置。"
LangString CageFinishText ${LANG_TRADCHINESE} "CCBox 已安裝。$\r$\n$\r$\n請使用同一個 Windows 帳戶開啟 CCBox，以繼續使用原有環境。如果意外出現首次設定，請先確認原儲存位置。"

LangString CageInstallLockFailed ${LANG_ENGLISH} "Could not lock the CCBox installation. Check your system permissions and try again."
LangString CageInstallLockFailed ${LANG_SIMPCHINESE} "无法锁定 CCBox 安装操作，请核对系统权限后重试。"
LangString CageInstallLockFailed ${LANG_TRADCHINESE} "無法鎖定 CCBox 安裝作業，請檢查系統權限後再試。"

LangString CageInstallBusy ${LANG_ENGLISH} "Another CCBox installation or uninstall is in progress. Wait for it to finish and try again."
LangString CageInstallBusy ${LANG_SIMPCHINESE} "另一个 CCBox 安装或卸载仍在进行，请等待完成后重试。"
LangString CageInstallBusy ${LANG_TRADCHINESE} "另一項 CCBox 安裝或解除安裝仍在進行，請等候完成後再試。"

LangString CageQuitBeforeInstall ${LANG_ENGLISH} "Quit CCBox and stop its environment before installing or uninstalling."
LangString CageQuitBeforeInstall ${LANG_SIMPCHINESE} "请先停止环境并退出 CCBox，再安装或卸载。"
LangString CageQuitBeforeInstall ${LANG_TRADCHINESE} "請先停止環境並結束 CCBox，再安裝或解除安裝。"

LangString CagePlatformRequired ${LANG_ENGLISH} "This CCBox package requires an Intel/AMD 64-bit version of Windows. Windows on ARM and 32-bit Windows are not supported."
LangString CagePlatformRequired ${LANG_SIMPCHINESE} "此 CCBox 安装包需要 Intel 或 AMD 64 位 Windows，不支持 Windows on ARM 或 32 位 Windows。"
LangString CagePlatformRequired ${LANG_TRADCHINESE} "此 CCBox 安裝套件需要 Intel 或 AMD 64 位元 Windows，不支援 Windows on ARM 或 32 位元 Windows。"

LangString CageWindowsRequired ${LANG_ENGLISH} "CCBox requires Windows 10 22H2 (build 19045) or Windows 11. Update Windows and try again."
LangString CageWindowsRequired ${LANG_SIMPCHINESE} "CCBox 需要 Windows 10 22H2（内部版本 19045）或 Windows 11。请更新 Windows 后重试。"
LangString CageWindowsRequired ${LANG_TRADCHINESE} "CCBox 需要 Windows 10 22H2（組建 19045）或 Windows 11。請更新 Windows 後再試。"

LangString CageChooseAppFolder ${LANG_ENGLISH} "Choose a dedicated application folder."
LangString CageChooseAppFolder ${LANG_SIMPCHINESE} "请选择独立的程序目录。"
LangString CageChooseAppFolder ${LANG_TRADCHINESE} "請選擇專用的應用程式資料夾。"

LangString CageFolderInUse ${LANG_ENGLISH} "The destination contains files from another installation. Choose an empty folder."
LangString CageFolderInUse ${LANG_SIMPCHINESE} "目标目录包含另一套安装的文件。请选择空文件夹。"
LangString CageFolderInUse ${LANG_TRADCHINESE} "目標資料夾包含另一套安裝的檔案。請選擇空資料夾。"

LangString CageRestorePending ${LANG_ENGLISH} "Installation recovery is incomplete. Keep both folders and try again: $BackupDir and $StageDir."
LangString CageRestorePending ${LANG_SIMPCHINESE} "安装恢复尚未完成。文件分别保留在 $BackupDir 和 $StageDir，请保留这两个目录后重试。"
LangString CageRestorePending ${LANG_TRADCHINESE} "安裝復原尚未完成。檔案分別保留在 $BackupDir 和 $StageDir，請保留這兩個資料夾後再試。"

LangString CageRestoreFailed ${LANG_ENGLISH} "The original installation folder could not be restored. Your files are in $BackupDir and the new application is in $StageDir."
LangString CageRestoreFailed ${LANG_SIMPCHINESE} "原安装目录未能恢复。文件保留在 $BackupDir，新程序保留在 $StageDir。"
LangString CageRestoreFailed ${LANG_TRADCHINESE} "無法復原原安裝資料夾。檔案保留在 $BackupDir，新應用程式保留在 $StageDir。"

LangString CageFilesReviewPending ${LANG_ENGLISH} "Some files still need review. Keep the installation record at $TransactionRecord and the related folders."
LangString CageFilesReviewPending ${LANG_SIMPCHINESE} "仍有文件需要核对，安装回执保留在 $TransactionRecord。请保留相关目录后处理。"
LangString CageFilesReviewPending ${LANG_TRADCHINESE} "仍有檔案需要確認。請保留位於 $TransactionRecord 的安裝紀錄及相關資料夾。"

LangString CagePendingTransaction ${LANG_ENGLISH} "An installation transaction is pending: $TransactionRecord. Restore the folders listed there and try again."
LangString CagePendingTransaction ${LANG_SIMPCHINESE} "存在未处理的安装事务记录：$TransactionRecord。请先按记录恢复保留的目录，再重试。"
LangString CagePendingTransaction ${LANG_TRADCHINESE} "有一筆尚未完成的安裝交易：$TransactionRecord。請先按照紀錄復原保留的資料夾，再試一次。"

LangString CageUnpackFailed ${LANG_ENGLISH} "Could not unpack CCBox. Check free disk space and folder permissions."
LangString CageUnpackFailed ${LANG_SIMPCHINESE} "无法解压 CCBox。请检查磁盘可用空间和目录权限。"
LangString CageUnpackFailed ${LANG_TRADCHINESE} "無法解壓縮 CCBox。請檢查磁碟可用空間和資料夾權限。"

LangString CageInstallingWebView ${LANG_ENGLISH} "Installing the bundled Microsoft WebView2 runtime..."
LangString CageInstallingWebView ${LANG_SIMPCHINESE} "正在安装随附的 Microsoft WebView2 运行时…"
LangString CageInstallingWebView ${LANG_TRADCHINESE} "正在安裝隨附的 Microsoft WebView2 執行階段…"

LangString CageWebViewFailed ${LANG_ENGLISH} "Microsoft WebView2 installation failed ($0). Install the runtime and retry."
LangString CageWebViewFailed ${LANG_SIMPCHINESE} "安装 Microsoft WebView2 失败（$0）。请安装运行时后重试。"
LangString CageWebViewFailed ${LANG_TRADCHINESE} "安裝 Microsoft WebView2 失敗（$0）。請安裝執行階段後再試。"

LangString CageCheckingFiles ${LANG_ENGLISH} "Checking and preserving existing projects and modified files…"
LangString CageCheckingFiles ${LANG_SIMPCHINESE} "正在检查并保留已有项目和修改文件…"
LangString CageCheckingFiles ${LANG_TRADCHINESE} "正在檢查並保留現有專案及修改過的檔案…"

LangString CagePreservationFailed ${LANG_ENGLISH} "Installation stopped because existing files could not be preserved safely. The original folder is intact. Check disk space, files in use, and the installation log."
LangString CagePreservationFailed ${LANG_SIMPCHINESE} "无法安全保留现有文件，安装已停止。原安装目录已保留，请检查磁盘空间、文件占用和安装日志。"
LangString CagePreservationFailed ${LANG_TRADCHINESE} "無法安全保留現有檔案，安裝已停止。原安裝資料夾已保留，請檢查磁碟空間、使用中的檔案和安裝日誌。"

LangString CageMoveFailed ${LANG_ENGLISH} "Could not move the installed application. Quit CCBox, close programs using files in its folder, and retry."
LangString CageMoveFailed ${LANG_SIMPCHINESE} "无法移动已安装的程序。请退出 CCBox 并关闭占用该目录文件的程序后重试。"
LangString CageMoveFailed ${LANG_TRADCHINESE} "無法移動已安裝的應用程式。請結束 CCBox 並關閉使用該資料夾檔案的程式後再試。"

LangString CageRestoringPersonalFiles ${LANG_ENGLISH} "Moving personal files and preserving existing links…"
LangString CageRestoringPersonalFiles ${LANG_SIMPCHINESE} "正在迁入个人文件并保留原有链接…"
LangString CageRestoringPersonalFiles ${LANG_TRADCHINESE} "正在移入個人檔案並保留現有連結…"

LangString CagePersonalFilesFailed ${LANG_ENGLISH} "Personal files could not be moved into the new application folder. The previous installation was restored. Close programs using those files and try again."
LangString CagePersonalFilesFailed ${LANG_SIMPCHINESE} "个人文件未能迁入新程序目录，原安装已恢复。请关闭占用文件的程序后重试。"
LangString CagePersonalFilesFailed ${LANG_TRADCHINESE} "無法將個人檔案移入新應用程式資料夾，原安裝已復原。請關閉使用這些檔案的程式後再試。"

LangString CageInstallRolledBack ${LANG_ENGLISH} "Could not install CCBox. The previous installation has been restored."
LangString CageInstallRolledBack ${LANG_SIMPCHINESE} "无法安装 CCBox。原安装已恢复。"
LangString CageInstallRolledBack ${LANG_TRADCHINESE} "無法安裝 CCBox。原安裝已復原。"

LangString CageInstallReviewRequired ${LANG_ENGLISH} "Installation did not finish. Keep $TransactionRecord and the folders listed there for review."
LangString CageInstallReviewRequired ${LANG_SIMPCHINESE} "安装未完成。请保留 $TransactionRecord 及其中记录的目录，以便核对。"
LangString CageInstallReviewRequired ${LANG_TRADCHINESE} "安裝未完成。請保留 $TransactionRecord 和其中列出的資料夾以供確認。"

LangString CageInstallFailed ${LANG_ENGLISH} "Could not install CCBox. Close other programs using the installation folder and retry."
LangString CageInstallFailed ${LANG_SIMPCHINESE} "无法安装 CCBox。请关闭占用安装目录的其他程序后重试。"
LangString CageInstallFailed ${LANG_TRADCHINESE} "無法安裝 CCBox。請關閉使用安裝資料夾的其他程式後再試。"

LangString CageCleaningFiles ${LANG_ENGLISH} "Removing verified files from the previous application…"
LangString CageCleaningFiles ${LANG_SIMPCHINESE} "正在清理已核对的旧程序文件…"
LangString CageCleaningFiles ${LANG_TRADCHINESE} "正在清理已確認的舊應用程式檔案…"

LangString CageRetainedFiles ${LANG_ENGLISH} "Files still need to be kept in the previous folder: $BackupDir"
LangString CageRetainedFiles ${LANG_SIMPCHINESE} "旧目录中仍有需要保留的文件：$BackupDir"
LangString CageRetainedFiles ${LANG_TRADCHINESE} "舊資料夾中仍有需要保留的檔案：$BackupDir"

LangString CageFilesRestoreRequired ${LANG_ENGLISH} "Some files still need to be restored. Keep $TransactionRecord and the folders listed there before proceeding."
LangString CageFilesRestoreRequired ${LANG_SIMPCHINESE} "安装仍有待恢复的文件。请保留 $TransactionRecord 及其中记录的目录，再处理恢复。"
LangString CageFilesRestoreRequired ${LANG_TRADCHINESE} "安裝中仍有待復原的檔案。請保留 $TransactionRecord 和其中列出的資料夾，再處理復原。"

LangString CageReceiptFailed ${LANG_ENGLISH} "The installation record could not be finalized. It is retained at $TransactionRecord."
LangString CageReceiptFailed ${LANG_SIMPCHINESE} "安装回执未能完成，记录保留在 $TransactionRecord。"
LangString CageReceiptFailed ${LANG_TRADCHINESE} "無法完成安裝紀錄，該紀錄保留在 $TransactionRecord。"

LangString CageInvalidInstallFolder ${LANG_ENGLISH} "This is not a CCBox installation folder."
LangString CageInvalidInstallFolder ${LANG_SIMPCHINESE} "这不是 CCBox 的安装目录。"
LangString CageInvalidInstallFolder ${LANG_TRADCHINESE} "這不是 CCBox 的安裝資料夾。"

LangString CageUninstallPendingTransaction ${LANG_ENGLISH} "An installation recovery is pending. First review the folders listed in $INSTDIR.ccbox-installation.json."
LangString CageUninstallPendingTransaction ${LANG_SIMPCHINESE} "存在待恢复的安装事务，请先处理 $INSTDIR.ccbox-installation.json 中记录的目录。"
LangString CageUninstallPendingTransaction ${LANG_TRADCHINESE} "有一筆待復原的安裝交易，請先處理 $INSTDIR.ccbox-installation.json 中記錄的資料夾。"

LangString CageUninstallFilesInUse ${LANG_ENGLISH} "Some application files are in use or cannot be read. Uninstall did not finish. Personal files are preserved. Close the programs using these files and try again."
LangString CageUninstallFilesInUse ${LANG_SIMPCHINESE} "部分程序文件正在使用或无法读取，卸载未完成。个人文件已保留，请关闭占用程序后重试。"
LangString CageUninstallFilesInUse ${LANG_TRADCHINESE} "部分應用程式檔案正在使用或無法讀取，解除安裝未完成。個人檔案已保留，請關閉使用這些檔案的程式後再試。"

Var UpdateMode
Var StageDir
Var BackupDir
Var ExistingInstall
Var DesktopShortcut
Var StartMenuShortcuts
Var DesktopShortcutControl
Var StartMenuShortcutsControl
Var InstallationMutex
Var TransactionRecord
Var DiagnosticFile
Var InstallPhase
Var InstallOutcome
Var HelperExit
Var DiagnosticArguments

Function WriteInstallDiagnostic
  ${If} $DiagnosticFile != ""
    Push $2
    Push $3
    StrCpy $3 0
    IfErrors 0 +2
      StrCpy $3 1
    ClearErrors
    FileOpen $2 "$DiagnosticFile" w
    ${IfNot} ${Errors}
      FileWrite $2 '{"phase":"$InstallPhase","status":"$InstallOutcome","helperExit":"$HelperExit"}$\r$\n'
      FileClose $2
    ${EndIf}
    ${If} $3 == 1
      SetErrors
    ${Else}
      ClearErrors
    ${EndIf}
    Pop $3
    Pop $2
  ${EndIf}
FunctionEnd

!macro InstallPhase NAME
  StrCpy $InstallPhase "${NAME}"
  Call WriteInstallDiagnostic
!macroend

Function un.WriteInstallDiagnostic
  ${If} $DiagnosticFile != ""
    Push $2
    Push $3
    StrCpy $3 0
    IfErrors 0 +2
      StrCpy $3 1
    ClearErrors
    FileOpen $2 "$DiagnosticFile" w
    ${IfNot} ${Errors}
      FileWrite $2 '{"phase":"$InstallPhase","status":"$InstallOutcome","helperExit":"$HelperExit"}$\r$\n'
      FileClose $2
    ${EndIf}
    ${If} $3 == 1
      SetErrors
    ${Else}
      ClearErrors
    ${EndIf}
    Pop $3
    Pop $2
  ${EndIf}
FunctionEnd

!macro UninstallPhase NAME
  StrCpy $InstallPhase "${NAME}"
  Call un.WriteInstallDiagnostic
!macroend

Function un.onUninstFailed
  StrCpy $InstallOutcome "failed"
  Call un.WriteInstallDiagnostic
FunctionEnd

Function un.onUninstSuccess
  StrCpy $InstallOutcome "success"
  !insertmacro UninstallPhase "completed"
FunctionEnd

Function .onInstFailed
  StrCpy $InstallOutcome "failed"
  Call WriteInstallDiagnostic
FunctionEnd

Function .onInstSuccess
  StrCpy $InstallOutcome "success"
  Call WriteInstallDiagnostic
FunctionEnd

!macro AcquireInstallationMutex
  ; NSIS and UpdateInstaller both reject an existing named object and keep
  ; their handle until the transaction ends. No caller waits for ownership.
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "Global\ccBox.Installation") p.r0 ?e'
  Pop $1
  ${If} $0 == 0
    SetErrorLevel 2
    Abort "$(CageInstallLockFailed)"
  ${EndIf}
  StrCpy $InstallationMutex $0
  ${If} $1 == 183
    SetErrorLevel 2
    Abort "$(CageInstallBusy)"
  ${EndIf}
!macroend

Function .onGUIEnd
  ${If} $InstallationMutex != ""
    System::Call 'kernel32::CloseHandle(p $InstallationMutex)'
  ${EndIf}
FunctionEnd

Function un.onGUIEnd
  ${If} $InstallationMutex != ""
    System::Call 'kernel32::CloseHandle(p $InstallationMutex)'
  ${EndIf}
FunctionEnd

Function LoadShortcutOptions
  ; Fresh installs enable both entries. Older installs inherit actual shortcuts.
  StrCpy $DesktopShortcut 1
  StrCpy $StartMenuShortcuts 1
  ReadRegStr $0 HKCU "${UNINSTALL_KEY}" "InstallLocation"
  ${If} $0 != ""
    StrCpy $DesktopShortcut 0
    StrCpy $StartMenuShortcuts 0
    ${If} ${FileExists} "$DESKTOP\CCBox.lnk"
      StrCpy $DesktopShortcut 1
    ${EndIf}
    ${If} ${FileExists} "$SMPROGRAMS\CCBox\CCBox.lnk"
      StrCpy $StartMenuShortcuts 1
    ${EndIf}
  ${EndIf}
  ClearErrors
  ReadRegDWORD $0 HKCU "${UNINSTALL_KEY}" "DesktopShortcut"
  ${IfNot} ${Errors}
    StrCpy $DesktopShortcut $0
  ${EndIf}
  ClearErrors
  ReadRegDWORD $0 HKCU "${UNINSTALL_KEY}" "StartMenuShortcuts"
  ${IfNot} ${Errors}
    StrCpy $StartMenuShortcuts $0
  ${EndIf}
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/DESKTOPSHORTCUT=" $1
  ${IfNot} ${Errors}
    ${If} $1 != "0"
    ${AndIf} $1 != "1"
      SetErrorLevel 2
      Abort "DESKTOPSHORTCUT must be 0 or 1."
    ${EndIf}
    StrCpy $DesktopShortcut $1
  ${EndIf}
  ClearErrors
  ${GetOptions} $0 "/STARTMENUSHORTCUT=" $1
  ${IfNot} ${Errors}
    ${If} $1 != "0"
    ${AndIf} $1 != "1"
      SetErrorLevel 2
      Abort "STARTMENUSHORTCUT must be 0 or 1."
    ${EndIf}
    StrCpy $StartMenuShortcuts $1
  ${EndIf}
FunctionEnd

Function ShortcutOptionsPage
  !insertmacro MUI_HEADER_TEXT "$(CageShortcutsTitle)" "$(CageShortcutsHelp)"
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateCheckbox} 0 8u 100% 16u "$(CageDesktopShortcut)"
  Pop $DesktopShortcutControl
  ${NSD_SetState} $DesktopShortcutControl $DesktopShortcut
  ${NSD_CreateCheckbox} 0 36u 100% 16u "$(CageStartMenuShortcuts)"
  Pop $StartMenuShortcutsControl
  ${NSD_SetState} $StartMenuShortcutsControl $StartMenuShortcuts
  ${NSD_OnBack} ShortcutOptionsLeave
  nsDialogs::Show
FunctionEnd

Function ShortcutOptionsLeave
  ${NSD_GetState} $DesktopShortcutControl $DesktopShortcut
  ${NSD_GetState} $StartMenuShortcutsControl $StartMenuShortcuts
FunctionEnd

Function SetDefaultInstallDirectory
  ; NSIS removes /D= from $CMDLINE, so inspect the original command line.
  System::Call 'kernel32::GetCommandLineW() w .r0'
  ClearErrors
  ${GetOptions} $0 "/D=" $1
  ${IfNot} ${Errors}
    Return
  ${EndIf}
  ; Keep upgrades in the registered location, including installations on C:.
  ReadRegStr $0 HKCU "${UNINSTALL_KEY}" "InstallLocation"
  ${If} $0 != ""
    StrCpy $INSTDIR $0
    Return
  ${EndIf}
  System::Call 'kernel32::GetDriveTypeW(w "D:\") i .r0'
  ${If} $0 == 3
    StrCpy $INSTDIR "D:\CCBox"
  ${EndIf}
FunctionEnd

!macro CheckStopped FILE
  ${If} ${FileExists} "$INSTDIR\${FILE}"
    ; Exclusive write access also detects images whose VM/controller has no window.
    ; Process exit can precede release of its executable's sharing lock.
    ; Retry sharing/lock violations briefly; never replace an image still in use.
    Push $1
    Push $2
    StrCpy $1 0
    ${Do}
      System::Call 'kernel32::CreateFileW(w "$INSTDIR\${FILE}", i 0x40000000, i 0, p 0, i 3, i 0, p 0) p .r0 ?e'
      Pop $2
      ${If} $0 != -1
        ${ExitDo}
      ${EndIf}
      ${If} $2 != 32
      ${AndIf} $2 != 33
        ${ExitDo}
      ${EndIf}
      IntOp $1 $1 + 1
      ${If} $1 >= 30
        ${ExitDo}
      ${EndIf}
      Sleep 100
    ${Loop}
    Pop $2
    Pop $1
    ${If} $0 == -1
      StrCpy $HelperExit "application_file_in_use"
      SetErrorLevel 2
      Abort "$(CageQuitBeforeInstall)"
    ${EndIf}
    System::Call 'kernel32::CloseHandle(p r0)'
  ${EndIf}
!macroend

Function .onInit
  StrCpy $InstallOutcome "running"
  StrCpy $HelperExit ""
  ${GetParameters} $0
  ${GetOptions} $0 "/DIAGNOSTICS=" $DiagnosticFile
  StrCpy $DiagnosticArguments ""
  ${If} $DiagnosticFile != ""
    StrCpy $DiagnosticArguments '-Diagnostics "$DiagnosticFile.helper.json"'
  ${EndIf}
  !insertmacro InstallPhase "initializing"
  SetShellVarContext current
  SetRegView 64
  ${IfNot} ${IsNativeAMD64}
    MessageBox MB_ICONSTOP "$(CagePlatformRequired)"
    SetErrorLevel 1633
    Quit
  ${EndIf}
  ReadRegStr $0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentBuildNumber"
  ${If} $0 < 19045
    MessageBox MB_ICONSTOP "$(CageWindowsRequired)"
    SetErrorLevel 1633
    Quit
  ${EndIf}
  Call SetDefaultInstallDirectory
  Call LoadShortcutOptions
  StrCpy $UpdateMode "0"
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/UPDATE" $1
  ${IfNot} ${Errors}
    StrCpy $UpdateMode "1"
    SetSilent silent
  ${EndIf}
FunctionEnd

Function RequireRegisteredUpgradeDirectory
  ReadRegStr $0 HKCU "${UNINSTALL_KEY}" "InstallLocation"
  ${If} $0 == ""
    Return
  ${EndIf}
  GetFullPathName $1 "$0"
  GetFullPathName $2 "$INSTDIR"
  ${If} $1 != $2
    StrCpy $HelperExit "registered_upgrade_directory_mismatch"
    SetErrorLevel 2
    Abort "$(CageUpgradeDirectoryError)"
  ${EndIf}
FunctionEnd

Function CheckDestination
  Call RequireRegisteredUpgradeDirectory
  ${GetRoot} "$INSTDIR" $0
  ${If} $0 == ""
  ${OrIf} $INSTDIR == $0
  ${OrIf} $INSTDIR == "$0\"
    SetErrorLevel 2
    Abort "$(CageChooseAppFolder)"
  ${EndIf}
  StrCpy $ExistingInstall "0"
  IfFileExists "$INSTDIR\ccBox.install-id" existing
  ; IfFileExists with a wildcard also matches an empty directory.
  ${DirState} "$INSTDIR" $0
  ${If} $0 == 1
    Goto foreign
  ${EndIf}
  ${If} $0 == -1
  ${AndIf} ${FileExists} "$INSTDIR"
    Goto foreign
  ${EndIf}
  Goto done
  existing:
    FileOpen $0 "$INSTDIR\ccBox.install-id" r
    FileRead $0 $1
    FileClose $0
    ${If} $1 != "${APP_ID}"
      Goto foreign
    ${EndIf}
    StrCpy $ExistingInstall "1"
    ; Never stop processes by their name or force-close user workloads.
    !insertmacro CheckStopped "Cage.exe"
    !insertmacro CheckStopped "resources\cage.exe"
    !insertmacro CheckStopped "resources\mihomo.exe"
    !insertmacro CheckStopped "resources\runtime\qemu\qemu-system-x86_64.exe"
    Goto done
  foreign:
    SetErrorLevel 2
    Abort "$(CageFolderInUse)"
  done:
    ClearErrors
FunctionEnd

Function RestorePreservedFiles
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation restore -Target "$BackupDir" -Source "$StageDir" -Receipt "$TransactionRecord" $DiagnosticArguments'
  Pop $0
  Pop $1
  ${If} $0 != 0
    DetailPrint "$1"
    SetErrorLevel 2
    Abort "$(CageRestorePending)"
  ${EndIf}
  ; The installation folder was kept, so the previous entries move back into it.
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation attach -Target "$INSTDIR" -Source "$BackupDir" $DiagnosticArguments'
  Pop $0
  Pop $1
  ${If} $0 != 0
    DetailPrint "$1"
    SetErrorLevel 2
    Abort "$(CageRestoreFailed)"
  ${EndIf}
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation complete -Target "$INSTDIR" -Receipt "$TransactionRecord" $DiagnosticArguments'
  Pop $0
  Pop $1
  ${If} $0 != 0
    SetErrorLevel 2
    Abort "$(CageFilesReviewPending)"
  ${EndIf}
FunctionEnd

Section "CCBox" Main
  !insertmacro InstallPhase "acquiring_installation_lock"
  !insertmacro AcquireInstallationMutex
  StrCpy $TransactionRecord "$INSTDIR.ccbox-installation.json"
  ${If} ${FileExists} "$TransactionRecord"
    SetErrorLevel 2
    Abort "$(CagePendingTransaction)"
  ${EndIf}
  !insertmacro InstallPhase "checking_destination"
  Call CheckDestination
  ${GetParent} "$INSTDIR" $0
  CreateDirectory "$0"
  GetTempFileName $StageDir "$0"
  Delete "$StageDir"
  CreateDirectory "$StageDir"
  !insertmacro InstallPhase "unpacking_application"
  SetOutPath "$StageDir"
  ClearErrors
!ifdef NSIS_WIN32_MAKENSIS
  File /r "${APP_DIR}\*"
!else
  File /r "${APP_DIR}/*"
!endif
  WriteUninstaller "$StageDir\Uninstall.exe"
  ${If} ${Errors}
    SetOutPath "$TEMP"
    RMDir /r "$StageDir"
    SetErrorLevel 2
    Abort "$(CageUnpackFailed)"
  ${EndIf}

  ; Install the bundled offline runtime only when WebView2 is absent.
  !insertmacro InstallPhase "detecting_webview_runtime"
  SetRegView 32
  ReadRegStr $0 HKLM "SOFTWARE\Microsoft\EdgeUpdate\Clients\${WEBVIEW2_APP_ID}" "pv"
  ReadRegStr $1 HKCU "Software\Microsoft\EdgeUpdate\Clients\${WEBVIEW2_APP_ID}" "pv"
  SetRegView 64
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ${If} $1 == ""
    ${OrIf} $1 == "0.0.0.0"
      DetailPrint "$(CageInstallingWebView)"
      !insertmacro InstallPhase "installing_webview_runtime"
      ExecWait '"$StageDir\resources\runtime\MicrosoftEdgeWebView2RuntimeInstallerX64.exe" /silent /install' $0
      StrCpy $HelperExit $0
      Call WriteInstallDiagnostic
      ${If} $0 != 0
        SetOutPath "$TEMP"
        RMDir /r "$StageDir"
        SetErrorLevel 2
        Abort "$(CageWebViewFailed)"
      ${EndIf}
    ${EndIf}
  ${EndIf}

  SetOutPath "$TEMP"
  StrCpy $BackupDir ""
  ${If} $ExistingInstall == "1"
    ${GetParent} "$INSTDIR" $0
    GetTempFileName $BackupDir "$0"
    Delete "$BackupDir"
  ${EndIf}
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=installation-files.ps1 "installation-files.ps1"
  File /oname=verified-file-deletion.cs "verified-file-deletion.cs"
  ; Keep NSIS's System.dll out of the PowerShell compiler working directory.
  SetOutPath "$TEMP"
  DetailPrint "$(CageCheckingFiles)"
  !insertmacro InstallPhase "preparing_file_transaction"
  ; Keep current and legacy data paths in place even if a VM holds a disk open.
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation prepare -Target "$StageDir" -Source "$INSTDIR" -Backup "$BackupDir" -Receipt "$TransactionRecord" -KeepApplicationData $DiagnosticArguments'
  Pop $0
  Pop $1
  StrCpy $HelperExit $0
  Call WriteInstallDiagnostic
  DetailPrint "$1"
  ${If} $0 != 0
    RMDir /r "$StageDir"
    SetErrorLevel 2
    Abort "$(CagePreservationFailed)"
  ${EndIf}
  ${If} $ExistingInstall == "1"
    !insertmacro InstallPhase "moving_previous_application"
    ; The installation folder itself stays; only its program and personal
    ; entries move, so kept data and open handles below it are untouched.
    nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation detach -Target "$BackupDir" -Source "$INSTDIR" -Receipt "$TransactionRecord" $DiagnosticArguments'
    Pop $0
    Pop $1
    StrCpy $HelperExit $0
    Call WriteInstallDiagnostic
    ${If} $0 != 0
      DetailPrint "$1"
      nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation complete -Target "$INSTDIR" -Receipt "$TransactionRecord" $DiagnosticArguments'
      Pop $0
      Pop $1
      RMDir /r "$StageDir"
      SetErrorLevel 2
      Abort "$(CageMoveFailed)"
    ${EndIf}
    DetailPrint "$(CageRestoringPersonalFiles)"
    !insertmacro InstallPhase "moving_personal_files"
    nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation move -Target "$StageDir" -Source "$BackupDir" -Receipt "$TransactionRecord" $DiagnosticArguments'
    Pop $0
    Pop $1
    StrCpy $HelperExit $0
    Call WriteInstallDiagnostic
    DetailPrint "$1"
    ${If} $0 != 0
      Call RestorePreservedFiles
      RMDir /r "$StageDir"
      SetErrorLevel 2
      Abort "$(CagePersonalFilesFailed)"
    ${EndIf}
  ${Else}
    RMDir "$INSTDIR"
  ${EndIf}
  !insertmacro InstallPhase "publishing_application"
  ClearErrors
  ${If} $BackupDir != ""
  ${OrIf} ${FileExists} "$INSTDIR\*.*"
    ; The kept installation folder receives the staged entries one by one.
    ; This also works when another process holds a pre-created empty folder open.
    StrCpy $2 ""
    ${If} $BackupDir == ""
      StrCpy $2 "-RequireEmptyDestination"
    ${EndIf}
    nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation attach -Target "$INSTDIR" -Source "$StageDir" $2 $DiagnosticArguments'
    Pop $0
    Pop $1
    StrCpy $HelperExit $0
    Call WriteInstallDiagnostic
    ${If} $0 != 0
      DetailPrint "$1"
      SetErrors
    ${EndIf}
  ${Else}
    Rename "$StageDir" "$INSTDIR"
  ${EndIf}
  ${If} ${Errors}
    ${If} $BackupDir != ""
      Call RestorePreservedFiles
      RMDir /r "$StageDir"
      SetErrorLevel 2
      Abort "$(CageInstallRolledBack)"
    ${EndIf}
    nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation abandon -Target "$INSTDIR" -Receipt "$TransactionRecord" $DiagnosticArguments'
    Pop $0
    Pop $1
    ${If} $0 != 0
      DetailPrint "$1"
      SetErrorLevel 2
      Abort "$(CageInstallReviewRequired)"
    ${EndIf}
    RMDir /r "$StageDir"
    SetErrorLevel 2
    Abort "$(CageInstallFailed)"
  ${EndIf}
  ${If} $BackupDir != ""
    !insertmacro InstallPhase "cleaning_previous_application"
    DetailPrint "$(CageCleaningFiles)"
    nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation cleanup -Target "$INSTDIR" -Source "$BackupDir" -Receipt "$TransactionRecord" $DiagnosticArguments'
    Pop $0
    Pop $1
    StrCpy $HelperExit $0
    Call WriteInstallDiagnostic
    DetailPrint "$1"
    ${If} $0 != 0
      DetailPrint "$(CageRetainedFiles)"
      SetErrorLevel 2
      Abort "$(CageFilesRestoreRequired)"
    ${EndIf}
  ${Else}
    !insertmacro InstallPhase "completing_file_transaction"
    nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation complete -Target "$INSTDIR" -Receipt "$TransactionRecord" $DiagnosticArguments'
    Pop $0
    Pop $1
    StrCpy $HelperExit $0
    Call WriteInstallDiagnostic
    ${If} $0 != 0
      SetErrorLevel 2
      Abort "$(CageReceiptFailed)"
    ${EndIf}
  ${EndIf}
  !insertmacro InstallPhase "registering_installation"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "CCBox"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "CCBox"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "URLInfoAbout" "https://ccbox.org/"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\Cage.exe,0"
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
  SetOutPath "$INSTDIR"
  ${If} $DesktopShortcut == 1
    CreateShortcut "$DESKTOP\CCBox.lnk" "$INSTDIR\Cage.exe"
  ${Else}
    Delete "$DESKTOP\CCBox.lnk"
  ${EndIf}
  ${If} $StartMenuShortcuts == 1
    CreateDirectory "$SMPROGRAMS\CCBox"
    CreateShortcut "$SMPROGRAMS\CCBox\CCBox.lnk" "$INSTDIR\Cage.exe"
    CreateShortcut "$SMPROGRAMS\CCBox\Uninstall CCBox.lnk" "$INSTDIR\Uninstall.exe"
  ${Else}
    Delete "$SMPROGRAMS\CCBox\CCBox.lnk"
    Delete "$SMPROGRAMS\CCBox\Uninstall CCBox.lnk"
    RMDir "$SMPROGRAMS\CCBox"
  ${EndIf}
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "DesktopShortcut" $DesktopShortcut
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "StartMenuShortcuts" $StartMenuShortcuts
  !insertmacro InstallPhase "completed"
  SetErrorLevel 0
SectionEnd

Function un.onInit
  StrCpy $InstallOutcome "running"
  StrCpy $HelperExit ""
  StrCpy $DiagnosticArguments ""
  ${GetParameters} $0
  ${GetOptions} $0 "/DIAGNOSTICS=" $DiagnosticFile
  ${If} $DiagnosticFile != ""
    StrCpy $DiagnosticArguments '-Diagnostics "$DiagnosticFile.helper.json"'
  ${EndIf}
  !insertmacro UninstallPhase "checking_installation_identity"
  SetShellVarContext current
  SetRegView 64
  FileOpen $0 "$INSTDIR\ccBox.install-id" r
  FileRead $0 $1
  FileClose $0
  ${If} $1 != "${APP_ID}"
    StrCpy $HelperExit "identity_mismatch"
    Call un.WriteInstallDiagnostic
    SetErrorLevel 2
    Abort "$(CageInvalidInstallFolder)"
  ${EndIf}
  !insertmacro UninstallPhase "checking_stopped_processes"
  !insertmacro CheckStopped "Cage.exe"
  !insertmacro CheckStopped "resources\cage.exe"
  !insertmacro CheckStopped "resources\mihomo.exe"
  !insertmacro CheckStopped "resources\runtime\qemu\qemu-system-x86_64.exe"
FunctionEnd

Section "Uninstall"
  !insertmacro UninstallPhase "acquiring_installation_lock"
  !insertmacro AcquireInstallationMutex
  !insertmacro UninstallPhase "checking_pending_transaction"
  ${If} ${FileExists} "$INSTDIR.ccbox-installation.json"
    StrCpy $HelperExit "pending_install_transaction"
    Call un.WriteInstallDiagnostic
    SetErrorLevel 2
    Abort "$(CageUninstallPendingTransaction)"
  ${EndIf}
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=installation-files.ps1 "installation-files.ps1"
  File /oname=verified-file-deletion.cs "verified-file-deletion.cs"
  File /oname=install-files.json "${FILE_MANIFEST}"
  ; PowerShell 5.1 Add-Type must not resolve NSIS's native System.dll as a
  ; managed compiler reference from the plugin working directory.
  SetOutPath "$TEMP"
  !insertmacro UninstallPhase "removing_owned_files"
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "$PLUGINSDIR\installation-files.ps1" -Operation uninstall -Target "$INSTDIR" -Manifest "$PLUGINSDIR\install-files.json" $DiagnosticArguments'
  Pop $0
  Pop $1
  StrCpy $HelperExit $0
  Call un.WriteInstallDiagnostic
  DetailPrint "$1"
  ${If} $0 != 0
    SetErrorLevel 2
    Abort "$(CageUninstallFilesInUse)"
  ${EndIf}
  !insertmacro UninstallPhase "removing_shortcuts_and_registration"
  Delete "$DESKTOP\CCBox.lnk"
  Delete "$SMPROGRAMS\CCBox\CCBox.lnk"
  Delete "$SMPROGRAMS\CCBox\Uninstall CCBox.lnk"
  RMDir "$SMPROGRAMS\CCBox"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  !insertmacro UninstallPhase "removing_install_directory"
  RMDir "$INSTDIR"
SectionEnd

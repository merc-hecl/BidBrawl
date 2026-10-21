Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma
Name "BidBrawl"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\BidBrawl"
InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "InstallLocation"
BrandingText " "
!include "MUI2.nsh"
!define MUI_FINISHPAGE_RUN "$INSTDIR\BidBrawl.exe"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "SimpChinese"

!macro CheckStopped
  IfFileExists "$INSTDIR\bin\bidbrawl-install.exe" 0 +5
  ExecWait '"$INSTDIR\bin\bidbrawl-install.exe" check' $0
  IntCmp $0 0 +3
  MessageBox MB_OK|MB_ICONEXCLAMATION "Please stop BidBrawl from its tray menu before continuing." /SD IDOK
  Abort
!macroend

Section
  !insertmacro CheckStopped
  SetOutPath "$INSTDIR"
  File "${SOURCE}\BidBrawl.exe"
  File "${SOURCE}\THIRD_PARTY_LICENSES.txt"
  File /r "${SOURCE}\bin"
  ExecWait '"$INSTDIR\bin\bidbrawl-install.exe" install' $0
  IntCmp $0 0 +3
  MessageBox MB_OK|MB_ICONSTOP "Could not register the bidbrawl command." /SD IDOK
  Abort
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortCut "$SMPROGRAMS\BidBrawl.lnk" "$INSTDIR\BidBrawl.exe"
  CreateShortCut "$DESKTOP\BidBrawl.lnk" "$INSTDIR\BidBrawl.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "DisplayName" "BidBrawl"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "DisplayIcon" "$INSTDIR\BidBrawl.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl" "NoRepair" 1
SectionEnd

Section "Uninstall"
  !insertmacro CheckStopped
  ExecWait '"$INSTDIR\bin\bidbrawl-install.exe" uninstall' $0
  IntCmp $0 0 +3
  MessageBox MB_OK|MB_ICONSTOP "Could not unregister the bidbrawl command." /SD IDOK
  Abort
  Delete "$SMPROGRAMS\BidBrawl.lnk"
  Delete "$DESKTOP\BidBrawl.lnk"
  Delete "$INSTDIR\BidBrawl.exe"
  Delete "$INSTDIR\THIRD_PARTY_LICENSES.txt"
  Delete "$INSTDIR\bin\bidbrawl.exe"
  Delete "$INSTDIR\bin\bidbrawl-install.exe"
  RMDir "$INSTDIR\bin"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\BidBrawl"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd

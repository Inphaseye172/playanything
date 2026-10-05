<#
.SYNOPSIS
  Removes PlayAnything from the current user profile.
      irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/uninstall.ps1 | iex
  Set $env:PLAYANYTHING_KEEP_CONFIG = "1" to keep config.json and the caches.
  mpv itself is left installed (it may be used by other programs); remove it with
  `winget uninstall shinchiro.mpv` if you installed it only for PlayAnything.
#>
$ErrorActionPreference = 'SilentlyContinue'
$Root   = Join-Path $env:LOCALAPPDATA 'PlayAnything'
$Bin    = Join-Path $Root 'bin'
$Exe    = Join-Path $Bin 'playanything.exe'
$ProgId = 'PlayAnything.Media'

Write-Host "Removing PlayAnything..." -ForegroundColor Cyan
if (Test-Path $Exe) { & $Exe stop | Out-Null }
Get-Process playanything -ErrorAction SilentlyContinue | Stop-Process -Force

$HKCU = [Microsoft.Win32.Registry]::CurrentUser
function Remove-RegKey([string]$Path) { try { $HKCU.DeleteSubKeyTree($Path, $false) } catch {} }
function Remove-RegValue([string]$Path, [string]$Name) {
    try { $k = $HKCU.OpenSubKey($Path, $true); if ($k) { $k.DeleteValue($Name, $false); $k.Close() } } catch {}
}
Remove-RegValue 'Software\Microsoft\Windows\CurrentVersion\Run' 'PlayAnything'
Remove-RegKey "Software\Classes\$ProgId"
Remove-RegKey 'Software\Classes\Applications\playanything.exe'
Remove-RegKey 'Software\Classes\*\shell\PlayAnything'
Remove-RegKey 'Software\Classes\*\shell\PlayAnythingAppend'
Remove-RegKey 'Software\Classes\Directory\shell\PlayAnything'
Remove-RegKey 'Software\Classes\Directory\Background\shell\PlayAnything'
Remove-RegKey 'Software\PlayAnything'
Remove-RegValue 'Software\RegisteredApplications' 'PlayAnything'
$classesKey = $HKCU.OpenSubKey('Software\Classes')
if ($classesKey) {
    foreach ($name in $classesKey.GetSubKeyNames()) {
        if ($name.StartsWith('.')) { Remove-RegValue "Software\Classes\$name\OpenWithProgids" $ProgId }
    }
    $classesKey.Close()
}
Remove-Item (Join-Path ([Environment]::GetFolderPath('Programs')) 'PlayAnything.lnk') -Force

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath) {
    $new = ($userPath -split ';' | Where-Object { $_ -and $_ -ne $Bin }) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $new, 'User')
}

if ($env:PLAYANYTHING_KEEP_CONFIG -eq '1') {
    Remove-Item -Recurse -Force $Bin
    Remove-Item -Recurse -Force (Join-Path $Root 'mpv-bin')
    Write-Host "Kept $Root\config.json and caches." -ForegroundColor Yellow
} else {
    Remove-Item -Recurse -Force $Root
}
try {
    Add-Type -Namespace PlayAnything -Name Shell -MemberDefinition '[DllImport("shell32.dll")] public static extern void SHChangeNotify(int eventId, int flags, IntPtr item1, IntPtr item2);'
    [PlayAnything.Shell]::SHChangeNotify(0x08000000, 0, [IntPtr]::Zero, [IntPtr]::Zero)
} catch {}
Write-Host "PlayAnything removed." -ForegroundColor Green

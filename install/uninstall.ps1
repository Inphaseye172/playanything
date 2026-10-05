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
$classes = 'HKCU:\Software\Classes'

Write-Host "Removing PlayAnything..." -ForegroundColor Cyan
if (Test-Path $Exe) { & $Exe stop | Out-Null }
Get-Process playanything -ErrorAction SilentlyContinue | Stop-Process -Force

Remove-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'PlayAnything'
Remove-Item -Recurse -Force "$classes\$ProgId"
Remove-Item -Recurse -Force "$classes\Applications\playanything.exe"
Remove-Item -Recurse -Force "$classes\*\shell\PlayAnything"
Remove-Item -Recurse -Force "$classes\*\shell\PlayAnythingAppend"
Remove-Item -Recurse -Force "$classes\Directory\shell\PlayAnything"
Remove-Item -Recurse -Force "$classes\Directory\Background\shell\PlayAnything"
Remove-Item -Recurse -Force 'HKCU:\Software\PlayAnything'
Remove-ItemProperty 'HKCU:\Software\RegisteredApplications' -Name 'PlayAnything'
Get-ChildItem $classes | Where-Object { $_.PSChildName -like '.*' } | ForEach-Object {
    $k = "$($_.PSPath)\OpenWithProgids"
    if (Test-Path $k) { Remove-ItemProperty $k -Name $ProgId }
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

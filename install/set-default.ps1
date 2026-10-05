<#
.SYNOPSIS
  Make PlayAnything the default app for every file type it supports (current user).

.DESCRIPTION
  Run in PowerShell (no admin needed) after installing PlayAnything:
      irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/set-default.ps1 | iex

  Windows 8/10/11 protect the per-user "default app" choice with a hash stored in
  HKCU\...\Explorer\FileExts\<ext>\UserChoice. Settings > Default apps writes it;
  nothing else is supposed to. This script computes the same hash, which is how
  SetUserFTA and PS-SFTA work, and writes a UserChoice for each of the 230+
  extensions PlayAnything registers. Afterwards a plain double-click opens
  PlayAnything, no "Open with" dance needed.

  Options (set before running):
    $env:PLAYANYTHING_SKIP  = ".jpg,.png,.gif"   # extensions to leave alone (comma separated)
    $env:PLAYANYTHING_ONLY  = "video,audio"      # limit to kinds: video, audio, image, raw, cinema, playlist
    $env:PLAYANYTHING_UNSET = "1"                # undo: remove our UserChoice entries so Windows asks again

.NOTES
  Hash algorithm: PS-SFTA by Danysys (https://github.com/DanysysTeam/PS-SFTA), MIT License,
  Copyright 2022 Danysys; credits to https://bbs.pediy.com/thread-213954.htm and LMongrain.
  The Get-Hash function below is vendored verbatim from SFTA.ps1 v1.2.0.

  Windows 11 24H2+ ships a "User Choice Protection Driver" (UCPD) that blocks
  scripted changes for http/https/.pdf. Media types are not affected, but if
  Windows later shows "An app default was reset", set that one type in
  Settings > Apps > Default apps > PlayAnything.
#>
& {
$ErrorActionPreference = 'Stop'

$ProgId = 'PlayAnything.Media'
function Step($m) { Write-Host "`n> $m" -ForegroundColor Cyan }
function Ok($m)   { Write-Host "  [ok] $m" -ForegroundColor Green }
function Warn($m) { Write-Host "  [!!] $m" -ForegroundColor Yellow }

# ---------------------------------------------------------------- locate PlayAnything
$Exe = Join-Path $env:LOCALAPPDATA 'PlayAnything\bin\playanything.exe'
if (-not (Test-Path $Exe)) {
    $c = Get-Command playanything.exe -ErrorAction SilentlyContinue
    if ($c) { $Exe = $c.Source }
}
if (-not (Test-Path $Exe)) {
    throw "PlayAnything is not installed. Run the installer first:`n  irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/install.ps1 | iex"
}
Ok "PlayAnything: $Exe"

# The ProgId must exist (the installer creates it); recreate the minimum if it is missing.
$HKCU = [Microsoft.Win32.Registry]::CurrentUser
if (-not $HKCU.OpenSubKey("Software\Classes\$ProgId\shell\open\command")) {
    $k = $HKCU.CreateSubKey("Software\Classes\$ProgId\shell\open\command", $true); $k.SetValue('', "`"$Exe`" `"%1`""); $k.Close()
    $k = $HKCU.CreateSubKey("Software\Classes\$ProgId", $true); $k.SetValue('', 'Media file'); $k.SetValue('FriendlyTypeName', 'Media file (PlayAnything)'); $k.Close()
    $k = $HKCU.CreateSubKey("Software\Classes\$ProgId\DefaultIcon", $true); $k.SetValue('', "`"$Exe`",0"); $k.Close()
    Warn "ProgId $ProgId was missing; recreated it"
}

# ---------------------------------------------------------------- which extensions
$all = (& $Exe extensions | Out-String) -split '\s+' | Where-Object { $_ }
if ($all.Count -lt 100) { throw "could not read the extension list from playanything.exe (got $($all.Count))" }

$kinds = @{}
if ($env:PLAYANYTHING_ONLY) {
    $want = ($env:PLAYANYTHING_ONLY -split ',') | ForEach-Object { $_.Trim().ToLower() } | Where-Object { $_ }
    foreach ($e in $all) {
        $info = (& $Exe info "x.$e" 2>$null | Out-String)   # kind line without touching any file
        $kind = if ($info -match 'kind:\s+(\S+)') { $Matches[1] } else { 'unknown' }
        $kinds[$e] = $kind
    }
    $map = @{ video = 'video'; audio = 'audio'; image = 'image'; raw = 'camera-raw'; cinema = 'redcode-r3d|blackmagic-raw'; playlist = 'playlist' }
    $pattern = ($want | ForEach-Object { $map[$_] } | Where-Object { $_ }) -join '|'
    $all = $all | Where-Object { $kinds[$_] -match "^($pattern)$" }
}
$skip = @()
if ($env:PLAYANYTHING_SKIP) { $skip = ($env:PLAYANYTHING_SKIP -split ',') | ForEach-Object { $_.Trim().TrimStart('.').ToLower() } | Where-Object { $_ } }
$exts = $all | Where-Object { $skip -notcontains $_ } | ForEach-Object { ".$_" }
if (-not $exts) { throw "nothing to do: every extension was filtered out" }

# ---------------------------------------------------------------- registry helpers
$code = @'
using System;
using System.Runtime.InteropServices;
namespace PlayAnythingReg {
  public class Utils {
    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    private static extern int RegOpenKeyEx(UIntPtr hKey, string subKey, int ulOptions, int samDesired, out UIntPtr hkResult);
    [DllImport("advapi32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    private static extern int RegDeleteKey(UIntPtr hKey, string subKey);
    [DllImport("advapi32.dll")]
    private static extern int RegCloseKey(UIntPtr hKey);
    [DllImport("shell32.dll")]
    private static extern void SHChangeNotify(int eventId, int flags, IntPtr item1, IntPtr item2);
    // The UserChoice key carries a Deny-SetValue ACE for the user, but deleting
    // and recreating it is allowed; that is what Settings itself does.
    public static int DeleteKey(string key) {
      UIntPtr h;
      if (RegOpenKeyEx((UIntPtr)0x80000001u, key, 0, 0x20019, out h) == 0) RegCloseKey(h);
      return RegDeleteKey((UIntPtr)0x80000001u, key);
    }
    public static void Refresh() { SHChangeNotify(0x08000000, 0, IntPtr.Zero, IntPtr.Zero); }
  }
}
'@
if (-not ('PlayAnythingReg.Utils' -as [type])) { Add-Type -TypeDefinition $code }

function Get-UserSid {
    try { return ([System.Security.Principal.WindowsIdentity]::GetCurrent()).User.Value.ToLower() }
    catch { return ((New-Object System.Security.Principal.NTAccount([Environment]::UserName)).Translate([System.Security.Principal.SecurityIdentifier]).Value).ToLower() }
}

# Windows embeds the exact "experience" string in shell32.dll; read it from there so the hash matches this build.
function Get-UserExperience {
    $hardcoded = 'User Choice set via Windows User Experience {D18B6DD5-6124-4341-9318-804003BAFA0B}'
    try {
        $path = [Environment]::GetFolderPath([Environment+SpecialFolder]::SystemX86) + '\Shell32.dll'
        $fs = [System.IO.File]::Open($path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
        $br = New-Object System.IO.BinaryReader($fs)
        [byte[]]$data = $br.ReadBytes(5mb)
        $fs.Close()
        $s = [Text.Encoding]::Unicode.GetString($data)
        $p1 = $s.IndexOf('User Choice set via Windows User Experience')
        if ($p1 -lt 0) { return $hardcoded }
        $p2 = $s.IndexOf('}', $p1)
        return $s.Substring($p1, $p2 - $p1 + 1)
    } catch { return $hardcoded }
}

# Windows validates the hash against the UserChoice key's last-write minute, so
# the timestamp must be taken right before the write and must not cross a minute.
function Get-HexDateTime {
    $now = [DateTime]::Now
    if ($now.Second -ge 57) { Start-Sleep -Seconds (61 - $now.Second); $now = [DateTime]::Now }
    $dt = New-Object DateTime($now.Year, $now.Month, $now.Day, $now.Hour, $now.Minute, 0)
    $ft = $dt.ToFileTime()
    $hi = ($ft -shr 32); $lo = ($ft -band 0xFFFFFFFFL)
    return ($hi.ToString('X8') + $lo.ToString('X8')).ToLower()
}

function Get-Hash {
  [CmdletBinding()]
  param (
    [Parameter( Position = 0, Mandatory = $True )]
    [string]
    $BaseInfo
  )


  function local:Get-ShiftRight {
    [CmdletBinding()]
    param (
      [Parameter( Position = 0, Mandatory = $true)]
      [long] $iValue, 
          
      [Parameter( Position = 1, Mandatory = $true)]
      [int] $iCount 
    )
  
    if ($iValue -band 0x80000000) {
      Write-Output (( $iValue -shr $iCount) -bxor 0xFFFF0000)
    }
    else {
      Write-Output  ($iValue -shr $iCount)
    }
  }
  

  function local:Get-Long {
    [CmdletBinding()]
    param (
      [Parameter( Position = 0, Mandatory = $true)]
      [byte[]] $Bytes,
  
      [Parameter( Position = 1)]
      [int] $Index = 0
    )
  
    Write-Output ([BitConverter]::ToInt32($Bytes, $Index))
  }
  

  function local:Convert-Int32 {
    param (
      [Parameter( Position = 0, Mandatory = $true)]
      [long] $Value
    )
  
    [byte[]] $bytes = [BitConverter]::GetBytes($Value)
    return [BitConverter]::ToInt32( $bytes, 0) 
  }

  [Byte[]] $bytesBaseInfo = [System.Text.Encoding]::Unicode.GetBytes($baseInfo) 
  $bytesBaseInfo += 0x00, 0x00  
  
  $MD5 = New-Object -TypeName System.Security.Cryptography.MD5CryptoServiceProvider
  [Byte[]] $bytesMD5 = $MD5.ComputeHash($bytesBaseInfo)
  
  $lengthBase = ($baseInfo.Length * 2) + 2 
  $length = (($lengthBase -band 4) -le 1) + (Get-ShiftRight $lengthBase  2) - 1
  $base64Hash = ""

  if ($length -gt 1) {
  
    $map = @{PDATA = 0; CACHE = 0; COUNTER = 0 ; INDEX = 0; MD51 = 0; MD52 = 0; OUTHASH1 = 0; OUTHASH2 = 0;
      R0 = 0; R1 = @(0, 0); R2 = @(0, 0); R3 = 0; R4 = @(0, 0); R5 = @(0, 0); R6 = @(0, 0); R7 = @(0, 0)
    }
  
    $map.CACHE = 0
    $map.OUTHASH1 = 0
    $map.PDATA = 0
    $map.MD51 = (((Get-Long $bytesMD5) -bor 1) + 0x69FB0000L)
    $map.MD52 = ((Get-Long $bytesMD5 4) -bor 1) + 0x13DB0000L
    $map.INDEX = Get-ShiftRight ($length - 2) 1
    $map.COUNTER = $map.INDEX + 1
  
    while ($map.COUNTER) {
      $map.R0 = Convert-Int32 ((Get-Long $bytesBaseInfo $map.PDATA) + [long]$map.OUTHASH1)
      $map.R1[0] = Convert-Int32 (Get-Long $bytesBaseInfo ($map.PDATA + 4))
      $map.PDATA = $map.PDATA + 8
      $map.R2[0] = Convert-Int32 (($map.R0 * ([long]$map.MD51)) - (0x10FA9605L * ((Get-ShiftRight $map.R0 16))))
      $map.R2[1] = Convert-Int32 ((0x79F8A395L * ([long]$map.R2[0])) + (0x689B6B9FL * (Get-ShiftRight $map.R2[0] 16)))
      $map.R3 = Convert-Int32 ((0xEA970001L * $map.R2[1]) - (0x3C101569L * (Get-ShiftRight $map.R2[1] 16) ))
      $map.R4[0] = Convert-Int32 ($map.R3 + $map.R1[0])
      $map.R5[0] = Convert-Int32 ($map.CACHE + $map.R3)
      $map.R6[0] = Convert-Int32 (($map.R4[0] * [long]$map.MD52) - (0x3CE8EC25L * (Get-ShiftRight $map.R4[0] 16)))
      $map.R6[1] = Convert-Int32 ((0x59C3AF2DL * $map.R6[0]) - (0x2232E0F1L * (Get-ShiftRight $map.R6[0] 16)))
      $map.OUTHASH1 = Convert-Int32 ((0x1EC90001L * $map.R6[1]) + (0x35BD1EC9L * (Get-ShiftRight $map.R6[1] 16)))
      $map.OUTHASH2 = Convert-Int32 ([long]$map.R5[0] + [long]$map.OUTHASH1)
      $map.CACHE = ([long]$map.OUTHASH2)
      $map.COUNTER = $map.COUNTER - 1
    }

    [Byte[]] $outHash = @(0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
    [byte[]] $buffer = [BitConverter]::GetBytes($map.OUTHASH1)
    $buffer.CopyTo($outHash, 0)
    $buffer = [BitConverter]::GetBytes($map.OUTHASH2)
    $buffer.CopyTo($outHash, 4)
  
    $map = @{PDATA = 0; CACHE = 0; COUNTER = 0 ; INDEX = 0; MD51 = 0; MD52 = 0; OUTHASH1 = 0; OUTHASH2 = 0;
      R0 = 0; R1 = @(0, 0); R2 = @(0, 0); R3 = 0; R4 = @(0, 0); R5 = @(0, 0); R6 = @(0, 0); R7 = @(0, 0)
    }
  
    $map.CACHE = 0
    $map.OUTHASH1 = 0
    $map.PDATA = 0
    $map.MD51 = ((Get-Long $bytesMD5) -bor 1)
    $map.MD52 = ((Get-Long $bytesMD5 4) -bor 1)
    $map.INDEX = Get-ShiftRight ($length - 2) 1
    $map.COUNTER = $map.INDEX + 1

    while ($map.COUNTER) {
      $map.R0 = Convert-Int32 ((Get-Long $bytesBaseInfo $map.PDATA) + ([long]$map.OUTHASH1))
      $map.PDATA = $map.PDATA + 8
      $map.R1[0] = Convert-Int32 ($map.R0 * [long]$map.MD51)
      $map.R1[1] = Convert-Int32 ((0xB1110000L * $map.R1[0]) - (0x30674EEFL * (Get-ShiftRight $map.R1[0] 16)))
      $map.R2[0] = Convert-Int32 ((0x5B9F0000L * $map.R1[1]) - (0x78F7A461L * (Get-ShiftRight $map.R1[1] 16)))
      $map.R2[1] = Convert-Int32 ((0x12CEB96DL * (Get-ShiftRight $map.R2[0] 16)) - (0x46930000L * $map.R2[0]))
      $map.R3 = Convert-Int32 ((0x1D830000L * $map.R2[1]) + (0x257E1D83L * (Get-ShiftRight $map.R2[1] 16)))
      $map.R4[0] = Convert-Int32 ([long]$map.MD52 * ([long]$map.R3 + (Get-Long $bytesBaseInfo ($map.PDATA - 4))))
      $map.R4[1] = Convert-Int32 ((0x16F50000L * $map.R4[0]) - (0x5D8BE90BL * (Get-ShiftRight $map.R4[0] 16)))
      $map.R5[0] = Convert-Int32 ((0x96FF0000L * $map.R4[1]) - (0x2C7C6901L * (Get-ShiftRight $map.R4[1] 16)))
      $map.R5[1] = Convert-Int32 ((0x2B890000L * $map.R5[0]) + (0x7C932B89L * (Get-ShiftRight $map.R5[0] 16)))
      $map.OUTHASH1 = Convert-Int32 ((0x9F690000L * $map.R5[1]) - (0x405B6097L * (Get-ShiftRight ($map.R5[1]) 16)))
      $map.OUTHASH2 = Convert-Int32 ([long]$map.OUTHASH1 + $map.CACHE + $map.R3) 
      $map.CACHE = ([long]$map.OUTHASH2)
      $map.COUNTER = $map.COUNTER - 1
    }
  
    $buffer = [BitConverter]::GetBytes($map.OUTHASH1)
    $buffer.CopyTo($outHash, 8)
    $buffer = [BitConverter]::GetBytes($map.OUTHASH2)
    $buffer.CopyTo($outHash, 12)
  
    [Byte[]] $outHashBase = @(0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
    $hashValue1 = ((Get-Long $outHash 8) -bxor (Get-Long $outHash))
    $hashValue2 = ((Get-Long $outHash 12) -bxor (Get-Long $outHash 4))
  
    $buffer = [BitConverter]::GetBytes($hashValue1)
    $buffer.CopyTo($outHashBase, 0)
    $buffer = [BitConverter]::GetBytes($hashValue2)
    $buffer.CopyTo($outHashBase, 4)
    $base64Hash = [Convert]::ToBase64String($outHashBase) 
  }

  Write-Output $base64Hash
}

# ---------------------------------------------------------------- do it
$sid = Get-UserSid
$experience = Get-UserExperience
$toasts = 'Software\Microsoft\Windows\CurrentVersion\ApplicationAssociationToasts'

if ($env:PLAYANYTHING_UNSET -eq '1') {
    Step "Removing PlayAnything as default for $($exts.Count) extensions"
    $n = 0
    foreach ($ext in $exts) {
        $uc = "Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\$ext\UserChoice"
        $k = $HKCU.OpenSubKey($uc)
        if ($k) {
            $cur = $k.GetValue('ProgId'); $k.Close()
            if ($cur -eq $ProgId) { [PlayAnythingReg.Utils]::DeleteKey($uc) | Out-Null; $n++ }
        }
    }
    [PlayAnythingReg.Utils]::Refresh()
    Ok "removed $n UserChoice entries; Windows will ask which app to use for those types"
    return
}

Step "Setting PlayAnything as the default for $($exts.Count) extensions"
$done = 0; $failed = @()
foreach ($ext in $exts) {
    try {
        # Remember the previous default so you can switch back in Settings; the toast entry stops the "new app" nag.
        $k = $HKCU.CreateSubKey($toasts, $true); $k.SetValue("${ProgId}_$ext", 0, [Microsoft.Win32.RegistryValueKind]::DWord); $k.Close()

        $stamp = Get-HexDateTime
        $baseInfo = "$ext$sid$ProgId$stamp$experience".ToLower()
        $hash = Get-Hash $baseInfo
        if (-not $hash) { throw "empty hash" }

        $uc = "Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\$ext\UserChoice"
        [PlayAnythingReg.Utils]::DeleteKey($uc) | Out-Null
        [Microsoft.Win32.Registry]::SetValue("HKEY_CURRENT_USER\$uc", 'Hash', $hash)
        [Microsoft.Win32.Registry]::SetValue("HKEY_CURRENT_USER\$uc", 'ProgId', $ProgId)
        $done++
    } catch {
        $failed += "$ext ($($_.Exception.Message))"
    }
}
[PlayAnythingReg.Utils]::Refresh()

# ---------------------------------------------------------------- verify
$verified = 0
foreach ($ext in $exts) {
    $k = $HKCU.OpenSubKey("Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\$ext\UserChoice")
    if ($k) { if ($k.GetValue('ProgId') -eq $ProgId) { $verified++ }; $k.Close() }
}
Ok "$done written, $verified verified as PlayAnything"
if ($failed) { Warn ("could not set: " + ($failed -join ', ')) }
Write-Host @"

  Double-clicking any of these files now opens PlayAnything.
  Undo:   `$env:PLAYANYTHING_UNSET='1'; irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/set-default.ps1 | iex
  Or pick another app for a single type in Settings > Apps > Default apps.
"@ -ForegroundColor Gray
}

<#
.SYNOPSIS
  PlayAnything one-line installer for Windows 10/11 (no admin rights needed).

.DESCRIPTION
  Run in PowerShell:
      irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/install.ps1 | iex

  What it does (everything lives under %LOCALAPPDATA%\PlayAnything):
    1. Downloads playanything.exe (the launcher) from the latest GitHub release
    2. Installs mpv, the playback engine, if it is not already present
       (winget shinchiro.mpv -> scoop -> chocolatey -> portable download)
    3. Writes PlayAnything's tuned mpv profile (GPU decode, HDR, network cache, RAW previews)
    4. Registers "Play with PlayAnything" in the right-click menu for every file and folder,
       adds PlayAnything to "Open with" for 230+ media extensions and to Settings > Default apps
    5. Adds a Start Menu shortcut and puts playanything on your PATH

  Options (set before running):
    $env:PLAYANYTHING_SERVICE  = "1"       # also run the background player at logon (single window, instant opens)
    $env:PLAYANYTHING_VERSION  = "v1.2.3"  # install a specific release instead of the latest
    $env:PLAYANYTHING_NO_ASSOC = "1"       # skip file associations / context menu
    $env:PLAYANYTHING_SET_DEFAULT = "1"    # open Settings > Default apps at the end so you can make it default

  Uninstall:
      irm https://raw.githubusercontent.com/inphaseye172/playanything/main/install/uninstall.ps1 | iex
#>
# Everything runs inside a script block so `irm | iex` leaves no variables,
# functions or preference changes behind in your session.
& {
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor 3072 } catch {}

$Repo   = 'inphaseye172/playanything'
$Root   = Join-Path $env:LOCALAPPDATA 'PlayAnything'
$Bin    = Join-Path $Root 'bin'
$Exe    = Join-Path $Bin 'playanything.exe'
$ProgId = 'PlayAnything.Media'

function Step($m) { Write-Host "`n> $m" -ForegroundColor Cyan }
function Ok($m)   { Write-Host "  [ok] $m" -ForegroundColor Green }
function Warn($m) { Write-Host "  [!!] $m" -ForegroundColor Yellow }

Write-Host "PlayAnything installer" -ForegroundColor White
Write-Host "Installing to $Root" -ForegroundColor DarkGray
New-Item -ItemType Directory -Force -Path $Bin | Out-Null

# ---------------------------------------------------------------- 1. launcher
$archRaw = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch  = if ($archRaw -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "playanything-windows-$arch.exe"
$ver   = $env:PLAYANYTHING_VERSION
$url   = if ($ver) { "https://github.com/$Repo/releases/download/$ver/$asset" } else { "https://github.com/$Repo/releases/latest/download/$asset" }

# A previous install may still be running (background player, open window):
# Windows cannot overwrite a running exe, but it can be renamed out of the way.
if (Test-Path $Exe) {
    try { & $Exe stop 2>$null | Out-Null } catch {}
    Get-Process playanything -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $Exe } | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 300
    try { Move-Item -Force $Exe "$Exe.old" } catch {}
    Remove-Item -Force "$Exe.old" -ErrorAction SilentlyContinue
}

Step "Downloading PlayAnything ($asset)"
$downloaded = $false
try {
    Invoke-WebRequest -Uri $url -OutFile "$Exe.tmp" -UseBasicParsing
    if ((Get-Item "$Exe.tmp").Length -lt 1MB) { throw "download too small" }
    Move-Item -Force "$Exe.tmp" $Exe
    Unblock-File $Exe -ErrorAction SilentlyContinue
    $downloaded = $true
    Ok "launcher: $Exe"
} catch {
    Remove-Item -Force "$Exe.tmp" -ErrorAction SilentlyContinue
    Warn "Release download failed: $($_.Exception.Message)"
}
if (-not $downloaded) {
    if (Get-Command go -ErrorAction SilentlyContinue) {
        Step "Building from source with Go"
        $oldCgo = $env:CGO_ENABLED; $oldGoBin = $env:GOBIN
        try {
            $env:CGO_ENABLED = '0'
            $env:GOBIN = $Bin
            & go install -ldflags '-s -w -H windowsgui' "github.com/$Repo/cmd/playanything@main"
        } finally { $env:CGO_ENABLED = $oldCgo; $env:GOBIN = $oldGoBin }
        if (-not (Test-Path $Exe)) { throw "go install did not produce $Exe" }
        Ok "built $Exe"
    } else {
        throw "Could not download $url and Go is not installed. Check https://github.com/$Repo/releases"
    }
}

# ---------------------------------------------------------------- 2. mpv
function Find-Mpv {
    $c = Get-Command mpv.exe -ErrorAction SilentlyContinue
    if ($c) { return $c.Source }
    $cands = @(
        "$Root\mpv-bin\mpv.exe",
        "$env:ProgramFiles\mpv\mpv.exe",
        "${env:ProgramFiles(x86)}\mpv\mpv.exe",
        "$env:LOCALAPPDATA\Programs\mpv\mpv.exe",
        "$env:USERPROFILE\scoop\apps\mpv\current\mpv.exe",
        "$env:ProgramData\chocolatey\bin\mpv.exe"
    )
    foreach ($p in $cands) { if ($p -and (Test-Path $p)) { return $p } }
    foreach ($dir in @((Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Packages'), (Join-Path $env:LOCALAPPDATA 'Programs'), $env:ProgramFiles, ${env:ProgramFiles(x86)})) {
        if ($dir -and (Test-Path $dir)) {
            $hit = Get-ChildItem -Path $dir -Filter mpv.exe -Recurse -Depth 3 -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($hit) { return $hit.FullName }
        }
    }
    return $null
}

Step "Checking for mpv (the playback engine)"
$mpv = Find-Mpv
if ($mpv) { Ok "found $mpv" }

if (-not $mpv -and (Get-Command winget -ErrorAction SilentlyContinue)) {
    Step "Installing mpv with winget (package shinchiro.mpv)"
    try {
        & winget install -e --id shinchiro.mpv --silent --accept-source-agreements --accept-package-agreements --disable-interactivity | Out-Host
    } catch { Warn "winget failed: $($_.Exception.Message)" }
    $mpv = Find-Mpv
}
if (-not $mpv -and (Get-Command scoop -ErrorAction SilentlyContinue)) {
    Step "Installing mpv with scoop"
    try { & scoop bucket add extras 2>$null | Out-Null; & scoop install mpv | Out-Host } catch { Warn "scoop failed: $($_.Exception.Message)" }
    $mpv = Find-Mpv
}
if (-not $mpv -and (Get-Command choco -ErrorAction SilentlyContinue)) {
    Step "Installing mpv with chocolatey"
    try { & choco install -y mpv | Out-Host } catch { Warn "choco failed: $($_.Exception.Message)" }
    $mpv = Find-Mpv
}
if (-not $mpv) {
    Step "Downloading a portable mpv build (shinchiro/mpv-winbuild-cmake)"
    try {
        $rel = Invoke-RestMethod 'https://api.github.com/repos/shinchiro/mpv-winbuild-cmake/releases/latest' -UseBasicParsing
        $pattern = if ($arch -eq 'arm64') { '^mpv-aarch64-\d{8}-git-[0-9a-f]+\.7z$' } else { '^mpv-x86_64-\d{8}-git-[0-9a-f]+\.7z$' }
        $a = $rel.assets | Where-Object { $_.name -match $pattern } | Select-Object -First 1
        if (-not $a) { throw "no matching asset in release $($rel.tag_name)" }
        $sevenZip = (Get-Command 7z.exe -ErrorAction SilentlyContinue | Select-Object -First 1).Source
        if (-not $sevenZip) {
            $sevenZip = Join-Path $Root '7zr.exe'
            Invoke-WebRequest 'https://www.7-zip.org/a/7zr.exe' -OutFile $sevenZip -UseBasicParsing
        }
        $archive = Join-Path $Root 'mpv.7z'
        Invoke-WebRequest $a.browser_download_url -OutFile $archive -UseBasicParsing
        New-Item -ItemType Directory -Force (Join-Path $Root 'mpv-bin') | Out-Null
        & $sevenZip x -y "-o$(Join-Path $Root 'mpv-bin')" $archive | Out-Null
        Remove-Item $archive -Force -ErrorAction SilentlyContinue
        $mpv = Find-Mpv
    } catch { Warn "portable download failed: $($_.Exception.Message)" }
}
if (-not $mpv) {
    throw "mpv could not be installed automatically. Install it from https://mpv.io/installation/ (or: winget install -e --id shinchiro.mpv) and run this installer again."
}
Ok "mpv: $mpv"

# ---------------------------------------------------------------- 3. PATH + profile
Step "Configuring"
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $userPath) { $userPath = '' }
if (($userPath -split ';') -notcontains $Bin) {
    [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ";$Bin").TrimStart(';'), 'User')
    Ok "added $Bin to your PATH (new terminals)"
}
$env:Path = "$env:Path;$Bin"

& $Exe setup | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
if (-not (Get-Command mpv.exe -ErrorAction SilentlyContinue)) {
    & $Exe config mpv_path "$mpv" | Out-Null
    Ok "mpv path saved to config.json"
}
if ($env:PLAYANYTHING_SERVICE -eq '1') {
    & $Exe config daemon always | Out-Null
}

# ---------------------------------------------------------------- 4. associations
if ($env:PLAYANYTHING_NO_ASSOC -ne '1') {
    Step "Registering file types and context menu (current user only)"
    $exts = (& $Exe extensions | Out-String) -split '\s+' | Where-Object { $_ }
    if ($exts.Count -lt 100) { throw "playanything.exe did not return its extension list (got $($exts.Count)); aborting registration" }
    $cmdOpen   = "`"$Exe`" `"%1`""
    $cmdAppend = "`"$Exe`" --append `"%1`""
    $iconRef   = "`"$Exe`",0"

    # Registry helpers via .NET: PowerShell's registry provider treats the
    # literal key name "*" (HKCU\Software\Classes\*) as a wildcard, so
    # New-Item/Set-ItemProperty cannot be used safely for context menus.
    $HKCU = [Microsoft.Win32.Registry]::CurrentUser
    function Set-RegValue([string]$Path, [string]$Name, [string]$Value) {
        $k = $HKCU.CreateSubKey($Path, $true)
        try { $k.SetValue($Name, $Value, [Microsoft.Win32.RegistryValueKind]::String) } finally { $k.Close() }
    }
    function Set-RegDefault([string]$Path, [string]$Value) { Set-RegValue $Path '' $Value }

    # ProgID used by "Open with" and Default apps
    $pid_ = "Software\Classes\$ProgId"
    Set-RegDefault "$pid_" 'Media file'
    Set-RegValue   "$pid_" 'FriendlyTypeName' 'Media file (PlayAnything)'
    Set-RegDefault "$pid_\DefaultIcon" $iconRef
    Set-RegDefault "$pid_\shell" 'open'
    Set-RegValue   "$pid_\shell\open" 'FriendlyAppName' 'PlayAnything'
    Set-RegValue   "$pid_\shell\open" 'Icon' $iconRef
    Set-RegDefault "$pid_\shell\open\command" $cmdOpen

    # Applications\playanything.exe so "Open with" shows a friendly name
    Set-RegValue   'Software\Classes\Applications\playanything.exe' 'FriendlyAppName' 'PlayAnything'
    Set-RegDefault 'Software\Classes\Applications\playanything.exe\shell\open\command' $cmdOpen

    # Default Programs capabilities (Settings > Apps > Default apps > PlayAnything)
    $cap = 'Software\PlayAnything\Capabilities'
    Set-RegValue $cap 'ApplicationName' 'PlayAnything'
    Set-RegValue $cap 'ApplicationDescription' 'One click, any media: video, audio, photos, camera RAW - local, NAS or cloud, GPU accelerated.'
    Set-RegValue $cap 'ApplicationIcon' $iconRef
    Set-RegValue 'Software\RegisteredApplications' 'PlayAnything' $cap

    $n = 0
    foreach ($e in $exts) {
        Set-RegValue "Software\Classes\.$e\OpenWithProgids" $ProgId ''
        Set-RegValue "$cap\FileAssociations" ".$e" $ProgId
        $n++
    }
    Ok "$n extensions registered for Open with / Default apps"

    # Right-click menu: every file, folders, folder background
    foreach ($base in @('Software\Classes\*\shell\PlayAnything', 'Software\Classes\Directory\shell\PlayAnything')) {
        Set-RegValue   $base 'MUIVerb' 'Play with PlayAnything'
        Set-RegValue   $base 'Icon' $iconRef
        Set-RegDefault "$base\command" $cmdOpen
    }
    Set-RegValue   'Software\Classes\*\shell\PlayAnythingAppend' 'MUIVerb' 'Add to PlayAnything playlist'
    Set-RegValue   'Software\Classes\*\shell\PlayAnythingAppend' 'Icon' $iconRef
    Set-RegDefault 'Software\Classes\*\shell\PlayAnythingAppend\command' $cmdAppend
    Set-RegValue   'Software\Classes\Directory\Background\shell\PlayAnything' 'MUIVerb' 'Play this folder with PlayAnything'
    Set-RegValue   'Software\Classes\Directory\Background\shell\PlayAnything' 'Icon' $iconRef
    Set-RegDefault 'Software\Classes\Directory\Background\shell\PlayAnything\command' "`"$Exe`" `"%V`""
    Ok "right-click 'Play with PlayAnything' added for files and folders"

    try {
        Add-Type -Namespace PlayAnything -Name Shell -MemberDefinition '[DllImport("shell32.dll")] public static extern void SHChangeNotify(int eventId, int flags, IntPtr item1, IntPtr item2);' -ErrorAction SilentlyContinue
        [PlayAnything.Shell]::SHChangeNotify(0x08000000, 0, [IntPtr]::Zero, [IntPtr]::Zero)
    } catch {}
}

# ---------------------------------------------------------------- 5. shortcuts, service
try {
    $sm = [Environment]::GetFolderPath('Programs')
    $ws = New-Object -ComObject WScript.Shell
    $sc = $ws.CreateShortcut((Join-Path $sm 'PlayAnything.lnk'))
    $sc.TargetPath = $Exe
    $sc.Description = 'PlayAnything - one click, any media'
    $sc.Save()
    Ok "Start Menu shortcut"
} catch { Warn "Start Menu shortcut skipped: $($_.Exception.Message)" }

if ($env:PLAYANYTHING_SERVICE -eq '1') {
    Step "Enabling the background player at logon"
    Set-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'PlayAnything' -Value "`"$Exe`" daemon"
    Start-Process -FilePath $Exe -ArgumentList 'daemon' -WindowStyle Hidden
    Ok "background player registered (HKCU\...\Run) and started"
}

# ---------------------------------------------------------------- 6. report
Step "Checking the installation"
& $Exe doctor | ForEach-Object { Write-Host "  $_" }

Write-Host "`nDone." -ForegroundColor Green
Write-Host @"

  * Right-click any media file or folder  ->  "Play with PlayAnything"
  * Or: right-click -> Open with -> PlayAnything (tick "Always")
  * To make it the default player for everything:
      Settings > Apps > Default apps > PlayAnything > "Set default"
  * Drag files onto the player window, or run:  playanything <file|folder|url>

"@ -ForegroundColor Gray
if ($env:PLAYANYTHING_SET_DEFAULT -eq '1') {
    Step "Making PlayAnything the default for every supported file type"
    Invoke-Expression (Invoke-RestMethod "https://raw.githubusercontent.com/$Repo/main/install/set-default.ps1" -UseBasicParsing)
}
}

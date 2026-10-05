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
$arch  = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "playanything-windows-$arch.exe"
$ver   = $env:PLAYANYTHING_VERSION
$url   = if ($ver) { "https://github.com/$Repo/releases/download/$ver/$asset" } else { "https://github.com/$Repo/releases/latest/download/$asset" }

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
        $env:CGO_ENABLED = '0'
        $env:GOBIN = $Bin
        & go install -ldflags '-s -w -H windowsgui' "github.com/$Repo/cmd/playanything@main"
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
    $wg = Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Packages'
    if (Test-Path $wg) {
        $cands += Get-ChildItem $wg -Filter mpv.exe -Recurse -ErrorAction SilentlyContinue | ForEach-Object FullName
    }
    foreach ($p in $cands) { if ($p -and (Test-Path $p)) { return $p } }
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
    $exts = (& $Exe extensions) -split '\s+' | Where-Object { $_ }
    $classes = 'HKCU:\Software\Classes'
    $cmdOpen   = "`"$Exe`" `"%1`""
    $cmdAppend = "`"$Exe`" --append `"%1`""

    # ProgID
    New-Item -Force "$classes\$ProgId\shell\open\command" | Out-Null
    New-Item -Force "$classes\$ProgId\DefaultIcon" | Out-Null
    Set-ItemProperty "$classes\$ProgId" -Name '(Default)' -Value 'Media file'
    Set-ItemProperty "$classes\$ProgId" -Name 'FriendlyTypeName' -Value 'Media file (PlayAnything)'
    Set-ItemProperty "$classes\$ProgId\DefaultIcon" -Name '(Default)' -Value "`"$Exe`",0"
    Set-ItemProperty "$classes\$ProgId\shell" -Name '(Default)' -Value 'open'
    Set-ItemProperty "$classes\$ProgId\shell\open" -Name 'FriendlyAppName' -Value 'PlayAnything'
    Set-ItemProperty "$classes\$ProgId\shell\open" -Name 'Icon' -Value "`"$Exe`",0"
    Set-ItemProperty "$classes\$ProgId\shell\open\command" -Name '(Default)' -Value $cmdOpen

    # Applications\playanything.exe so "Open with" shows a friendly name
    New-Item -Force "$classes\Applications\playanything.exe\shell\open\command" | Out-Null
    Set-ItemProperty "$classes\Applications\playanything.exe" -Name 'FriendlyAppName' -Value 'PlayAnything'
    Set-ItemProperty "$classes\Applications\playanything.exe\shell\open\command" -Name '(Default)' -Value $cmdOpen

    # Default Programs capabilities (Settings > Apps > Default apps > PlayAnything)
    $cap = 'HKCU:\Software\PlayAnything\Capabilities'
    New-Item -Force "$cap\FileAssociations" | Out-Null
    Set-ItemProperty $cap -Name 'ApplicationName' -Value 'PlayAnything'
    Set-ItemProperty $cap -Name 'ApplicationDescription' -Value 'One click, any media: video, audio, photos, camera RAW - local, NAS or cloud, GPU accelerated.'
    Set-ItemProperty $cap -Name 'ApplicationIcon' -Value "`"$Exe`",0"
    New-Item -Force 'HKCU:\Software\RegisteredApplications' | Out-Null
    Set-ItemProperty 'HKCU:\Software\RegisteredApplications' -Name 'PlayAnything' -Value 'Software\PlayAnything\Capabilities'

    $n = 0
    foreach ($e in $exts) {
        $key = "$classes\.$e\OpenWithProgids"
        New-Item -Force $key | Out-Null
        Set-ItemProperty $key -Name $ProgId -Value ''
        Set-ItemProperty "$cap\FileAssociations" -Name ".$e" -Value $ProgId
        $n++
    }
    Ok "$n extensions registered for Open with / Default apps"

    # Right-click menu: every file, folders, folder background
    foreach ($base in @("$classes\*\shell\PlayAnything", "$classes\Directory\shell\PlayAnything")) {
        New-Item -Force "$base\command" | Out-Null
        Set-ItemProperty $base -Name 'MUIVerb' -Value 'Play with PlayAnything'
        Set-ItemProperty $base -Name 'Icon' -Value "`"$Exe`",0"
        Set-ItemProperty "$base\command" -Name '(Default)' -Value $cmdOpen
    }
    New-Item -Force "$classes\*\shell\PlayAnythingAppend\command" | Out-Null
    Set-ItemProperty "$classes\*\shell\PlayAnythingAppend" -Name 'MUIVerb' -Value 'Add to PlayAnything playlist'
    Set-ItemProperty "$classes\*\shell\PlayAnythingAppend" -Name 'Icon' -Value "`"$Exe`",0"
    Set-ItemProperty "$classes\*\shell\PlayAnythingAppend\command" -Name '(Default)' -Value $cmdAppend
    New-Item -Force "$classes\Directory\Background\shell\PlayAnything\command" | Out-Null
    Set-ItemProperty "$classes\Directory\Background\shell\PlayAnything" -Name 'MUIVerb' -Value 'Play this folder with PlayAnything'
    Set-ItemProperty "$classes\Directory\Background\shell\PlayAnything" -Name 'Icon' -Value "`"$Exe`",0"
    Set-ItemProperty "$classes\Directory\Background\shell\PlayAnything\command" -Name '(Default)' -Value "`"$Exe`" `"%V`""
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
if ($env:PLAYANYTHING_SET_DEFAULT -eq '1') { Start-Process 'ms-settings:defaultapps' }

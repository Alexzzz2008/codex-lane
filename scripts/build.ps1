param(
    [string]$Version = "",
    [switch]$NoBump
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$serviceFile = Join-Path $root "internal/appservice/service.go"
$infoFile = Join-Path $root "build/windows/info.json"

if (($env:PATHEXT -split ";") -notcontains ".EXE") {
    $env:PATHEXT = ".COM;.EXE;.BAT;.CMD;.VBS;.VBE;.JS;.JSE;.WSF;.WSH;.MSC"
}

function Assert-WindowsGui([string]$Path) {
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $reader = New-Object System.IO.BinaryReader($stream)
        $stream.Position = 0x3C
        $peOffset = $reader.ReadInt32()
        $stream.Position = $peOffset + 24 + 68
        $subsystem = $reader.ReadUInt16()
        if ($subsystem -ne 2) {
            throw "Expected Windows GUI subsystem (2), got $subsystem for $Path"
        }
    }
    finally {
        $stream.Dispose()
    }
}

$match = Select-String -Path $serviceFile -Pattern 'const Version = "([^"]+)"' | Select-Object -First 1
if (-not $match) {
    throw "Cannot find the application version in $serviceFile"
}
$current = $match.Matches[0].Groups[1].Value

if ($Version) {
    $target = $Version
} elseif ($NoBump) {
    $target = $current
} else {
    $parts = $current.Split(".")
    while ($parts.Count -lt 3) { $parts += "0" }
    $patch = [int]$parts[2] + 1
    $target = "$($parts[0]).$($parts[1]).$patch"
}

if ($target -notmatch '^\d+\.\d+\.\d+$') {
    throw "Version must use MAJOR.MINOR.PATCH, got '$target'"
}

Write-Host "Version: $current -> $target"
$utf8Read = New-Object System.Text.UTF8Encoding($false)
$originalServiceText = [System.IO.File]::ReadAllText($serviceFile, $utf8Read)
$originalInfoText = [System.IO.File]::ReadAllText($infoFile, $utf8Read)
$serviceText = $originalServiceText
$serviceText = $serviceText -replace 'const Version = "[^"]+"', "const Version = `"$target`""
$utf8 = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($serviceFile, $serviceText, $utf8)

$infoText = $originalInfoText
$infoText = $infoText -replace '"file_version":\s*"[^"]+"', "`"file_version`": `"$target.0`""
$infoText = $infoText -replace '"product_version":\s*"[^"]+"', "`"product_version`": `"$target.0`""
$infoText = $infoText -replace '"ProductVersion":\s*"[^"]+"', "`"ProductVersion`": `"$target`""
[System.IO.File]::WriteAllText($infoFile, $infoText, $utf8)

try {
foreach ($tool in @("npm", "wails3", "go")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "$tool is required and was not found in PATH"
    }
}

Push-Location (Join-Path $root "frontend")
try {
    if (Test-Path "node_modules/.bin/vite.cmd") {
        npm run check
        if ($LASTEXITCODE -ne 0) { throw "npm run check failed with exit code $LASTEXITCODE" }
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "npm run build failed with exit code $LASTEXITCODE" }
    }
    else {
        $wsl = Get-Command wsl.exe -ErrorAction SilentlyContinue
        if (-not $wsl) {
            throw "Windows npm shims are missing and WSL is unavailable; reinstall frontend dependencies"
        }
        $drive = $root.Substring(0, 1).ToLowerInvariant()
        $frontendPath = "/mnt/$drive" + $root.Substring(2).Replace('\', '/') + "/frontend"
        wsl.exe bash -lc "cd '$frontendPath' && npm run check && npm run build"
        if ($LASTEXITCODE -ne 0) { throw "WSL npm build failed with exit code $LASTEXITCODE" }
    }
}
finally {
    Pop-Location
}

Push-Location $root
try {
    wails3 generate bindings -clean=true -ts -i -names -time-type=string
    if ($LASTEXITCODE -ne 0) { throw "wails3 generate bindings failed with exit code $LASTEXITCODE" }
    Push-Location "build"
    try {
        wails3 generate syso -arch amd64 -icon windows/icon.ico `
            -manifest windows/wails.exe.manifest -info windows/info.json `
            -out ../wails_windows_amd64.syso
        if ($LASTEXITCODE -ne 0) { throw "wails3 generate syso failed with exit code $LASTEXITCODE" }
    }
    finally {
        Pop-Location
    }

    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    $versionedExe = Join-Path $root "bin/CodexLane-$target.exe"
    go build -tags production -trimpath -buildvcs=false `
        -ldflags="-w -s -H windowsgui" -o $versionedExe .
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
    Assert-WindowsGui $versionedExe
}
finally {
    Pop-Location
}
}
catch {
    [System.IO.File]::WriteAllText($serviceFile, $originalServiceText, $utf8)
    [System.IO.File]::WriteAllText($infoFile, $originalInfoText, $utf8)
    Write-Host "Build failed; version rolled back to $current"
    throw
}

$exe = $versionedExe
$liveExe = Join-Path $root "bin/CodexLane.exe"
try {
    Copy-Item $exe $liveExe -Force
}
catch {
    Write-Host "Live exe is in use; kept the current process and packaged the versioned build."
}
$size = [math]::Round((Get-Item $exe).Length / 1MB, 2)

$desktop = [Environment]::GetFolderPath("Desktop")
if (-not $desktop -or -not (Test-Path $desktop)) {
    $desktop = Join-Path $env:USERPROFILE "Desktop"
}
$archive = Join-Path $desktop "CodexLane-$target-win-x64.zip"
$stage = Join-Path ([System.IO.Path]::GetTempPath()) ("codexlane-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $stage -Force | Out-Null
try {
    Copy-Item $exe (Join-Path $stage "CodexLane.exe")
    $hash = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLowerInvariant()
    Set-Content -Path (Join-Path $stage "SHA256.txt") -Value "$hash  CodexLane.exe" -Encoding ascii
    Remove-Item $archive -Force -ErrorAction SilentlyContinue
    Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $archive -CompressionLevel Optimal
}
finally {
    Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Built CodexLane $target ($size MB)"
Write-Host "  exe: $exe"
Write-Host "  zip: $archive"
Write-Host "  sha256: $hash"

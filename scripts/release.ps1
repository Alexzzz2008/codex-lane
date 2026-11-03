param(
    [string]$Version = "",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$serviceFile = Join-Path $root "internal/appservice/service.go"

if (-not $Version) {
    $match = Select-String -Path $serviceFile -Pattern 'const Version = "([^"]+)"' | Select-Object -First 1
    if (-not $match) {
        throw "Cannot find the application version in $serviceFile"
    }
    $Version = $match.Matches[0].Groups[1].Value
}

$infoFile = Join-Path $root "build/windows/info.json"
$info = Get-Content $infoFile -Raw | ConvertFrom-Json
if ($info.info.'0409'.ProductVersion -ne $Version) {
    throw "build/windows/info.json is $($info.info.'0409'.ProductVersion), expected $Version"
}

if (-not $SkipBuild) {
    & (Join-Path $PSScriptRoot "build.ps1") -NoBump
    if ($LASTEXITCODE -ne 0) { throw "build failed with exit code $LASTEXITCODE" }
}

$exe = Join-Path $root "bin/CodexLane-$Version.exe"
if (-not (Test-Path $exe)) {
    $exe = Join-Path $root "bin/CodexLane.exe"
}
if (-not (Test-Path $exe)) {
    throw "Missing build output: $exe"
}

$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Path $dist -Force | Out-Null
$releaseExeName = "CodexLane-windows-amd64.exe"
$releaseExe = Join-Path $dist $releaseExeName
Copy-Item $exe $releaseExe -Force
$exeHash = (Get-FileHash $releaseExe -Algorithm SHA256).Hash.ToLowerInvariant()
$exeSize = (Get-Item $releaseExe).Length

$archiveName = "CodexLane-$Version-win-x64.zip"
$archive = Join-Path $dist $archiveName
Remove-Item $archive -Force -ErrorAction SilentlyContinue
$stage = Join-Path ([System.IO.Path]::GetTempPath()) ("codexlane-release-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $stage -Force | Out-Null
try {
    Copy-Item $releaseExe (Join-Path $stage "CodexLane.exe")
    Set-Content -Path (Join-Path $stage "SHA256.txt") -Value "$exeHash  CodexLane.exe" -Encoding ascii
    Compress-Archive -Path (Join-Path $stage "*") -DestinationPath $archive -CompressionLevel Optimal
}
finally {
    Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
}

$archiveHash = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
$checksumFile = Join-Path $dist "SHA256SUMS.txt"
"$exeHash  $releaseExeName`n$archiveHash  $archiveName" | Set-Content $checksumFile -Encoding ascii

$tag = "v$Version"
$releaseURL = "https://github.com/Alexzzz2008/codex-lane/releases/tag/$tag"
$downloadURL = "https://github.com/Alexzzz2008/codex-lane/releases/download/$tag/$releaseExeName"
$manifest = [ordered]@{
    version = $Version
    notes = ""
    releaseUrl = $releaseURL
    publishedAt = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    assets = [ordered]@{
        "windows-amd64" = [ordered]@{
            url = $downloadURL
            size = $exeSize
            sha256 = $exeHash
        }
    }
}
$manifestFile = Join-Path $dist "latest.json"
$utf8 = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($manifestFile, ($manifest | ConvertTo-Json -Depth 8), $utf8)

$size = [math]::Round((Get-Item $archive).Length / 1MB, 2)
Write-Host ""
Write-Host "Release ready"
Write-Host "  update exe: $releaseExe"
Write-Host "  archive:  $archive"
Write-Host "  checksum: $checksumFile"
Write-Host "  manifest: $manifestFile"
Write-Host "  size:     $size MB"
Write-Host "  exe sha256: $exeHash"
Write-Host "  zip sha256: $archiveHash"

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

$exe = Join-Path $root "bin/CodexLane.exe"
if (-not (Test-Path $exe)) {
    throw "Missing build output: $exe"
}

$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Path $dist -Force | Out-Null
$archiveName = "CodexLane-$Version-win-x64.zip"
$archive = Join-Path $dist $archiveName
Remove-Item $archive -Force -ErrorAction SilentlyContinue
Compress-Archive -Path $exe -DestinationPath $archive -CompressionLevel Optimal

$checksum = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
$checksumFile = Join-Path $dist "SHA256SUMS.txt"
"$checksum  $archiveName" | Set-Content $checksumFile -Encoding ascii

$size = [math]::Round((Get-Item $archive).Length / 1MB, 2)
Write-Host ""
Write-Host "Release ready"
Write-Host "  archive:  $archive"
Write-Host "  checksum: $checksumFile"
Write-Host "  size:     $size MB"
Write-Host "  sha256:   $checksum"

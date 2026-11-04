param([Parameter(Mandatory=$true)][string]$Executable)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName UIAutomationClient
Add-Type -AssemblyName UIAutomationTypes
Add-Type -AssemblyName System.Drawing
Add-Type @'
using System;
using System.Runtime.InteropServices;
public class LaneSmokeWindow {
    [StructLayout(LayoutKind.Sequential)] public struct RECT { public int Left, Top, Right, Bottom; }
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int command);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")] public static extern bool IsZoomed(IntPtr h);
    [DllImport("user32.dll")] public static extern bool IsIconic(IntPtr h);
    [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint message, IntPtr w, IntPtr l);
    [DllImport("dwmapi.dll")] public static extern int DwmGetWindowAttribute(IntPtr h, int attribute, out RECT rect, int size);
}
'@
[LaneSmokeWindow]::SetProcessDPIAware() | Out-Null
$root = Split-Path -Parent $PSScriptRoot
$directory = Join-Path $root ("bin/native-qa-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $directory | Out-Null
$target = Join-Path $directory "CodexLane.exe"
Copy-Item $Executable $target
Copy-Item $Executable ($target + ".old")
$marker = Join-Path $directory ".codexlane-ready-smoke"
$utf8 = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText($target + ".update.json", (@{backup=($target+".old");marker=$marker;installerPid=0} | ConvertTo-Json), $utf8)
$homeDirectory = Join-Path $directory "home"
New-Item -ItemType Directory -Path $homeDirectory | Out-Null
$fixture = @{
    schemaVersion=1; upstreamProxy=@{protocol="http";host="127.0.0.1";port=7897;noProxy="localhost,127.0.0.1,::1"};
    engine="windows";startup=@{promptCodexProxy=$false;openCodex=$false;stopCodexOnExit=$false;minimizeToTray=$true};
    logging=@{requestMetadata=$true;retentionDays=14;maxMemoryRows=1000}
}
[IO.File]::WriteAllText((Join-Path $homeDirectory "launcher.json"), ($fixture | ConvertTo-Json -Depth 5), $utf8)

function Start-IsolatedLane {
    $info = New-Object Diagnostics.ProcessStartInfo
    $info.FileName = $target
    $info.UseShellExecute = $false
    $info.EnvironmentVariables["CODEXLANE_HOME"] = $homeDirectory
    $info.EnvironmentVariables["CODEX_HOME"] = (Join-Path $directory "codex-config")
    $info.EnvironmentVariables["CODEXLANE_NO_MIGRATE"] = "1"
    $info.EnvironmentVariables["CODEXLANE_UPDATE_FEED"] = "http://127.0.0.1:1/disabled.json"
    return [Diagnostics.Process]::Start($info)
}
function Wait-Condition([scriptblock]$Condition, [string]$Message, [int]$Seconds=30) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    do {
        if (& $Condition) { return }
        Start-Sleep -Milliseconds 100
    } while ((Get-Date) -lt $deadline)
    throw $Message
}
function Invoke-Page([string]$Name) {
    $automation = [Windows.Automation.AutomationElement]::FromHandle($script:windowHandle)
    $condition = New-Object Windows.Automation.PropertyCondition([Windows.Automation.AutomationElement]::NameProperty, $Name)
    $element = $automation.FindFirst([Windows.Automation.TreeScope]::Descendants, $condition)
    if (-not $element) { throw "Missing native UI control: $Name" }
    $element.GetCurrentPattern([Windows.Automation.InvokePattern]::Pattern).Invoke()
    Start-Sleep -Milliseconds 350
}
function Capture-Window([string]$Name) {
    [LaneSmokeWindow]::SetForegroundWindow($script:windowHandle) | Out-Null
    Start-Sleep -Milliseconds 300
    $rect = New-Object LaneSmokeWindow+RECT
    if ([LaneSmokeWindow]::DwmGetWindowAttribute($script:windowHandle, 9, [ref]$rect, [Runtime.InteropServices.Marshal]::SizeOf($rect)) -ne 0) { throw "Cannot measure window" }
    $image = New-Object Drawing.Bitmap ($rect.Right-$rect.Left), ($rect.Bottom-$rect.Top)
    $graphics = [Drawing.Graphics]::FromImage($image)
    try {
        $graphics.CopyFromScreen($rect.Left, $rect.Top, 0, 0, $image.Size)
        $image.Save((Join-Path $directory ($Name+".png")), [Drawing.Imaging.ImageFormat]::Png)
    } finally { $graphics.Dispose(); $image.Dispose() }
}
$process = Start-IsolatedLane
try {
    Wait-Condition { $process.Refresh(); $process.MainWindowHandle -ne [IntPtr]::Zero } "Isolated window did not appear"
    $script:windowHandle = $process.MainWindowHandle
    Wait-Condition { -not (Test-Path ($target+".update.json")) } "Updated UI did not confirm readiness"
    $configuration = Get-Content (Join-Path $homeDirectory "launcher.json") -Raw | ConvertFrom-Json
    try {
        $proxyURL = "http://127.0.0.1:" + $configuration.lane.interfacePort
        Invoke-WebRequest -UseBasicParsing -Uri "https://auth.openai.com/.well-known/openid-configuration" -Proxy $proxyURL -TimeoutSec 15 | Out-Null
        Write-Output "Public OAuth discovery request completed through isolated Lane."
    } catch { Write-Output "Public probe: $($_.Exception.Message)" }
    Start-Sleep -Seconds 8
    foreach ($page in @("总览", "请求日志", "检查", "设置")) { Invoke-Page $page; Capture-Window $page }
    $before = $process.TotalProcessorTime.TotalMilliseconds
    Start-Sleep -Seconds 2
    $process.Refresh()
    $idleCPU = [math]::Round(($process.TotalProcessorTime.TotalMilliseconds-$before)/2000*100, 2)
    Write-Output "Isolated main-process idle CPU (one core): $idleCPU%"
    [LaneSmokeWindow]::ShowWindow($script:windowHandle, 3) | Out-Null
    Wait-Condition { [LaneSmokeWindow]::IsZoomed($script:windowHandle) } "Maximize failed" 5
    [LaneSmokeWindow]::ShowWindow($script:windowHandle, 9) | Out-Null
    [LaneSmokeWindow]::ShowWindow($script:windowHandle, 6) | Out-Null
    Wait-Condition { [LaneSmokeWindow]::IsIconic($script:windowHandle) } "Minimize failed" 5
    [LaneSmokeWindow]::ShowWindow($script:windowHandle, 9) | Out-Null
    [LaneSmokeWindow]::PostMessage($script:windowHandle, 0x10, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
    Wait-Condition { -not [LaneSmokeWindow]::IsWindowVisible($script:windowHandle) } "Close-to-tray failed" 5
    if ($process.HasExited) { throw "Close-to-tray exited the application" }
    $second = Start-IsolatedLane
    Wait-Condition { [LaneSmokeWindow]::IsWindowVisible($script:windowHandle) } "Second launch did not restore the window" 10
    if (-not $second.WaitForExit(10000)) { $second.Kill(); throw "Second instance did not exit" }
    $configuration = Get-Content (Join-Path $homeDirectory "launcher.json") -Raw | ConvertFrom-Json
    foreach ($port in @($configuration.lane.interfacePort, $configuration.lane.kernelPort)) {
        $health = Invoke-RestMethod ("http://127.0.0.1:"+$port+"/healthz") -TimeoutSec 3
        if (-not $health) { throw "Local endpoint did not respond" }
    }
    Write-Output "PASS: isolated readiness recovery, four native pages, maximize/minimize/restore, close-to-tray, single instance, and both loopback endpoints."
    Write-Output "Screenshots: $directory"
} finally {
    if (-not $process.HasExited) { $process.Kill(); $process.WaitForExit() }
}

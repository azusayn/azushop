# Start kubectl port-forward + ngrok for the Azushop Envoy gateway.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File misc/expose.ps1
# Output: JSON to stdout and expose-result.json in the current directory.

param(
    [string]$Namespace = "azushop",
    [int]$LocalPort = 18000,
    [int]$RemotePort = 10000
)

$ErrorActionPreference = "Continue"

function Resolve-Tool([string]$Name) {
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    throw "command not found: $Name"
}

$kubectl = Resolve-Tool kubectl
$ngrok = Resolve-Tool ngrok

Get-Process ngrok -ErrorAction SilentlyContinue | Stop-Process -Force
Get-CimInstance Win32_Process -Filter "Name='kubectl.exe'" |
    Where-Object { $_.CommandLine -like "*port-forward*$Namespace*envoy*" } |
    ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep -Seconds 2

$pf = Start-Process -FilePath $kubectl -ArgumentList @(
    "port-forward", "-n", $Namespace, "svc/envoy", "${LocalPort}:${RemotePort}"
) -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 8

$ng = Start-Process -FilePath $ngrok -ArgumentList @("http", "$LocalPort") -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 12

$result = @{
    public_url = $null
    stripe_url = $null
    port_forward_pid = $pf.Id
    ngrok_pid = $ng.Id
    error = $null
}

try {
    $tunnels = Invoke-RestMethod -Uri "http://127.0.0.1:4040/api/tunnels" -TimeoutSec 15
    $https = $tunnels.tunnels | Where-Object { $_.public_url -like "https*" } | Select-Object -First 1
    if ($https) {
        $result.public_url = $https.public_url
        $result.stripe_url = "$($https.public_url)/v1/payment/callback/stripe"
    } else {
        $result.error = "no_https_tunnel"
    }
} catch {
    $result.error = $_.Exception.Message
}

$outFile = Join-Path (Get-Location) "expose-result.json"
$result | ConvertTo-Json | Set-Content -Path $outFile -Encoding UTF8
Write-Output ($result | ConvertTo-Json -Compress)

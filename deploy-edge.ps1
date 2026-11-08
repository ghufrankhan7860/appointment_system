<#
.SYNOPSIS
  Ship the edge Caddy (deploy/edge) to the EC2 host and start or reload it.

.DESCRIPTION
  The edge Caddy is the only container on the host that publishes 80/443. It serves
  medappoint.in (production) and staging.medappoint.in (staging) and reaches each
  stack's api and web by alias on the shared appointment-edge network. Staging and
  production deploys never touch it; run this only when a domain or route changes.

  Steps:
    1. Pre-flight checks (tools, key, files, SSH reachable)
    2. Upload compose.edge.yaml, the Caddyfile and a small server-side script with scp
    3. On the server: create appointment-edge if missing, validate the new Caddyfile
       in a throwaway container, refuse if another container holds 80/443, then
       start Caddy or reload it in place (no dropped connections)
    4. Optional health checks against URLs

  No secrets: the Caddyfile names the domains and Caddy keeps its certificates in
  the caddy_data volume.

.EXAMPLE
  .\deploy-edge.ps1
  .\deploy-edge.ps1 -HealthUrls @("https://medappoint.in/health", "https://staging.medappoint.in/health")

  If Windows blocks the script:
  powershell -ExecutionPolicy Bypass -File .\deploy-edge.ps1
#>
[CmdletBinding()]
param(
    [string]$ServerHost = "13.55.81.62",
    [string]$User       = "ubuntu",
    [string]$KeyPath    = "$HOME\.ssh\appointment-staging.pem",
    [string]$RemoteDir  = "/home/ubuntu/edge",
    [string]$LocalDir   = "deploy/edge",
    [string]$Network    = "appointment-edge",

    [string[]]$HealthUrls = @()
)

$ErrorActionPreference = "Stop"
$script:StepNo   = 0
$script:Started  = Get-Date
$Remote          = "$User@$ServerHost"
$CommonSshOpts   = @("-i", $KeyPath, "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=30")
$RemoteWorkDir   = "$RemoteDir/.deploy"
$LocalWorkDir    = Join-Path $env:TEMP "appointment-deploy-edge"
$ComposeName     = "compose.edge.yaml"

# ---------- output helpers ----------
function Write-Step([string]$Message) {
    $script:StepNo++
    Write-Host ""
    Write-Host ("==> [{0}] Step {1}: {2}" -f (Get-Date -Format "HH:mm:ss"), $script:StepNo, $Message) -ForegroundColor Cyan
}
function Write-Info([string]$Message) { Write-Host "    $Message" -ForegroundColor Gray }
function Write-Ok([string]$Message)   { Write-Host "    OK   $Message" -ForegroundColor Green }
function Stop-Deploy([string]$Message) {
    Write-Host ""
    Write-Host "DEPLOY FAILED: $Message" -ForegroundColor Red
    Write-Host ("Stopped after {0:N0}s." -f ((Get-Date) - $script:Started).TotalSeconds) -ForegroundColor Red
    exit 1
}

# Runs a native command, shows it, streams its output, times it, stops on failure.
function Invoke-Native {
    param([string]$Exe, [string[]]$ArgList, [string]$What = $Exe)
    Write-Host ("    > {0} {1}" -f $Exe, ($ArgList -join " ")) -ForegroundColor DarkGray
    $sw = [Diagnostics.Stopwatch]::StartNew()
    & $Exe @ArgList
    $code = $LASTEXITCODE
    $sw.Stop()
    if ($code -ne 0) { Stop-Deploy "$What failed (exit code $code)." }
    Write-Ok ("{0} finished in {1:N1}s" -f $What, $sw.Elapsed.TotalSeconds)
}

# ---------- 1. pre-flight ----------
Write-Step "Pre-flight checks"

foreach ($tool in @("ssh", "scp")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { Stop-Deploy "'$tool' was not found on PATH." }
    Write-Ok "$tool found"
}

if (-not (Test-Path $KeyPath)) { Stop-Deploy "SSH key not found at $KeyPath" }
Write-Ok "SSH key found: $KeyPath"

$localCompose   = Join-Path $LocalDir $ComposeName
$localCaddyfile = Join-Path $LocalDir "Caddyfile"
foreach ($file in @($localCompose, $localCaddyfile)) {
    if (-not (Test-Path $file)) { Stop-Deploy "'$file' not found. Run this from the backend repo root." }
    Write-Ok "found $file"
}

Write-Info "Checking SSH connection to $Remote ..."
& ssh @CommonSshOpts $Remote "echo connected to `$(hostname)"
if ($LASTEXITCODE -ne 0) {
    Stop-Deploy "Cannot SSH to $Remote. Check the instance is running and the security group allows SSH from your current IP."
}
Write-Ok "SSH works"

# ---------- 2. upload ----------
Write-Step "Preparing folders on the server"
Invoke-Native -Exe "ssh" -ArgList ($CommonSshOpts + @($Remote, "mkdir -p $RemoteWorkDir")) -What "remote mkdir"

# Server-side script. Written with LF line endings so bash can run it.
$remoteScript = @'
set -euo pipefail
step() { printf '\n--> [server %s] %s\n' "$(date +%H:%M:%S)" "$1"; }

cd "__REMOTE_DIR__"
compose() { docker compose -f __COMPOSE__ "$@"; }

step "Making sure the shared network __NETWORK__ exists"
docker network inspect "__NETWORK__" >/dev/null 2>&1 || docker network create "__NETWORK__"

step "Validating the new Caddyfile"
# A throwaway container, so a broken file never reaches the running Caddy.
docker run --rm -v "__WORK_DIR__/Caddyfile:/etc/caddy/Caddyfile:ro" caddy:2.8 \
    caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile

step "Checking nothing else holds ports 80 and 443"
for port in 80 443; do
    holders=$(docker ps --filter "publish=$port" --format '{{.Names}} {{.Label "com.docker.compose.project"}}' | grep -v ' appointment-edge$' || true)
    if [ -n "$holders" ]; then
        echo "Port $port is published by: $holders"
        echo "Deploy that stack without its Caddy first (deploy-staging.ps1 removes the old staging Caddy)."
        exit 1
    fi
done

step "Installing the new files"
# cp writes into the existing files, so the bind-mounted Caddyfile keeps its inode
# and the running container sees the new content.
cp "__WORK_DIR__/__COMPOSE__" __COMPOSE__
cp "__WORK_DIR__/Caddyfile" Caddyfile
rm -f "__WORK_DIR__/__COMPOSE__" "__WORK_DIR__/Caddyfile"

step "Starting or reloading Caddy"
running=$(compose ps --status running --quiet caddy)
compose up -d
if [ -n "$running" ]; then
    if compose exec -T caddy cat /etc/caddy/Caddyfile | cmp -s - Caddyfile; then
        compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
    else
        echo "The container does not see the new Caddyfile; recreating it."
        compose up -d --force-recreate caddy
    fi
fi

step "Container status"
compose ps

step "Last 20 log lines"
compose logs --tail=20 --no-color
'@
$remoteScript = $remoteScript.Replace("__REMOTE_DIR__", $RemoteDir).
                              Replace("__WORK_DIR__", $RemoteWorkDir).
                              Replace("__COMPOSE__", $ComposeName).
                              Replace("__NETWORK__", $Network)
$remoteScript = $remoteScript -replace "`r", ""
New-Item -ItemType Directory -Force $LocalWorkDir | Out-Null
$localRemoteScript = Join-Path $LocalWorkDir "remote-deploy-edge.sh"
[IO.File]::WriteAllText($localRemoteScript, $remoteScript, (New-Object Text.UTF8Encoding($false)))

Write-Step "Uploading the compose file, Caddyfile and server script"
Invoke-Native -Exe "scp" -ArgList ($CommonSshOpts + @($localCompose, $localCaddyfile, $localRemoteScript, "${Remote}:$RemoteWorkDir/")) -What "upload"

# ---------- 3. run on the server ----------
Write-Step "Validating and starting the edge Caddy on the server"
Invoke-Native -Exe "ssh" -ArgList ($CommonSshOpts + @($Remote, "bash $RemoteWorkDir/remote-deploy-edge.sh")) -What "remote deploy"

# ---------- 4. health checks ----------
foreach ($url in $HealthUrls) {
    Write-Step "Health check: $url"
    $healthy = $false
    for ($i = 1; $i -le 12; $i++) {
        try {
            $resp = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 10
            if ($resp.StatusCode -eq 200) {
                Write-Ok "HTTP 200 on attempt $i"
                $healthy = $true
                break
            }
            Write-Info "attempt ${i}: HTTP $($resp.StatusCode)"
        } catch {
            Write-Info "attempt ${i}: $($_.Exception.Message)"
        }
        Start-Sleep -Seconds 5
    }
    if (-not $healthy) {
        Stop-Deploy "Health check of $url did not pass after 12 attempts. Check logs: ssh -i $KeyPath $Remote 'cd $RemoteDir && docker compose -f $ComposeName logs --tail=100'"
    }
}

# ---------- summary ----------
$total = (Get-Date) - $script:Started
Write-Host ""
Write-Host ("EDGE DEPLOY SUCCEEDED in {0:N0}s" -f $total.TotalSeconds) -ForegroundColor Green
Write-Host "Logs: ssh -i $KeyPath $Remote `"cd $RemoteDir && docker compose -f $ComposeName logs -f`"" -ForegroundColor Gray

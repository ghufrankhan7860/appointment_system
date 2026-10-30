<#
.SYNOPSIS
  Build Docker images on this PC, ship them to the EC2 staging server,
  load them there and (re)start the stack with Docker Compose.

.DESCRIPTION
  Steps:
    1. Pre-flight checks (tools, key, Docker running, SSH reachable)
    2. Build each image locally for linux/amd64
    3. Save all images into one .tar file
    4. Upload the .tar, the env file and a small server-side script with scp
    5. Sync the local deploy/ and migrations/ folders to the server
    6. On the server: install the env file as deploy/.env (mode 600), load images,
       docker compose up, show status and logs, clean up
    7. Optional health check against a URL

  Secrets live in -EnvFile (default envs/.env.staging, git-ignored by the .env.*
  rule), never in this script. Only the variable names are checked and printed.

.EXAMPLE
  .\deploy-staging.ps1
  .\deploy-staging.ps1 -HealthUrl https://52-65-56-135.sslip.io/health
  .\deploy-staging.ps1 -SkipBuild            # re-ship images already built
  .\deploy-staging.ps1 -SkipConfigSync       # don't overwrite server deploy/ files

  Multiple images built from Dockerfile stages:
  .\deploy-staging.ps1 -Images @(
      @{ Tag = "appointment-api:staging";      Target = "api" },
      @{ Tag = "appointment-consumer:staging"; Target = "consumer" }
  )

  If Windows blocks the script:
  powershell -ExecutionPolicy Bypass -File .\deploy-staging.ps1
#>
[CmdletBinding()]
param(
    [string]$ServerHost  = "52.65.56.135",
    [string]$User        = "ubuntu",
    [string]$KeyPath     = "$HOME\.ssh\appointment-staging.pem",
    [string]$RemoteDir   = "/home/ubuntu/app",
    [string]$LocalDeployDir = "deploy",
    [string]$ComposeFile = "deploy/compose.staging.yaml",
    [string]$Dockerfile  = "deploy/Dockerfile",
    [string]$EnvFile     = "envs/.env.staging",
    [string]$Platform    = "linux/amd64",

    # Tag = image name used in the compose file; Target = Dockerfile stage ("" = whole Dockerfile)
    [hashtable[]]$Images = @(
        @{ Tag = "appointment-api:staging"; Target = "" }
    ),

    [string]$HealthUrl = "",
    [switch]$SkipBuild,
    [switch]$SkipConfigSync
)

$ErrorActionPreference = "Stop"
$script:StepNo   = 0
$script:Started  = Get-Date
$Remote          = "$User@$ServerHost"
$CommonSshOpts   = @("-i", $KeyPath, "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=30")
$TarName         = "appointment-images.tar"
$TarPath         = Join-Path $env:TEMP $TarName
$RemoteWorkDir   = "$RemoteDir/.deploy"

# ---------- output helpers ----------
function Write-Step([string]$Message) {
    $script:StepNo++
    Write-Host ""
    Write-Host ("==> [{0}] Step {1}: {2}" -f (Get-Date -Format "HH:mm:ss"), $script:StepNo, $Message) -ForegroundColor Cyan
}
function Write-Info([string]$Message) { Write-Host "    $Message" -ForegroundColor Gray }
function Write-Ok([string]$Message)   { Write-Host "    OK   $Message" -ForegroundColor Green }
function Write-Warn([string]$Message) { Write-Host "    WARN $Message" -ForegroundColor Yellow }
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

# Runs a native command silently and reports success. Windows PowerShell 5.1
# turns redirected stderr into a terminating error under "Stop", so relax it here.
function Test-Native([string]$Exe, [string[]]$ArgList) {
    $ErrorActionPreference = "Continue"
    & $Exe @ArgList *> $null
    return ($LASTEXITCODE -eq 0)
}

# Variables the staging stack cannot start without.
# DB_HOST, DB_PORT and DB_SSLMODE are fixed by compose.staging.yaml.
$RequiredEnv = @("APP_ENV", "JWT_SECRET", "DB_USER", "DB_PASSWORD", "DB_NAME",
                 "PAYMENT_PROVIDER", "SITE_ADDRESS")

# ---------- 1. pre-flight ----------
Write-Step "Pre-flight checks"

foreach ($tool in @("docker", "ssh", "scp")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { Stop-Deploy "'$tool' was not found on PATH." }
    Write-Ok "$tool found"
}

if (-not (Test-Path $KeyPath)) { Stop-Deploy "SSH key not found at $KeyPath" }
Write-Ok "SSH key found: $KeyPath"

if (-not $SkipConfigSync) {
    if (-not (Test-Path $LocalDeployDir)) { Stop-Deploy "Local '$LocalDeployDir' folder not found. Run this from the repo root." }
    if (-not (Test-Path $ComposeFile))    { Stop-Deploy "Compose file '$ComposeFile' not found." }
    Write-Ok "Compose file found: $ComposeFile"
    # scp -r would copy a local deploy/.env over the server's.
    if (Test-Path (Join-Path $LocalDeployDir ".env")) { Stop-Deploy "Remove '$LocalDeployDir/.env'; staging secrets come from -EnvFile." }
}
if (-not $SkipBuild -and -not (Test-Path $Dockerfile)) { Stop-Deploy "Dockerfile '$Dockerfile' not found." }

if (-not (Test-Path $EnvFile)) { Stop-Deploy "Env file '$EnvFile' not found. Copy envs/.env.example to it and fill in staging values." }
$envNames = @{}
foreach ($line in Get-Content $EnvFile) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$' -and $Matches[2].Trim()) { $envNames[$Matches[1]] = $true }
}
$missingEnv = @($RequiredEnv | Where-Object { -not $envNames.ContainsKey($_) })
if ($missingEnv.Count -gt 0) { Stop-Deploy "Env file '$EnvFile' has no value for: $($missingEnv -join ', ')" }
Write-Ok "Env file found with $($envNames.Count) variables: $EnvFile"

if (-not (Test-Native "docker" @("info"))) { Stop-Deploy "Docker is not running. Start Docker Desktop and try again." }
Write-Ok "Docker is running"

$GitSha = "unknown"
try {
    $GitSha = (git rev-parse --short HEAD 2>$null)
    if (-not $GitSha) { $GitSha = "unknown" }
    $dirty = git status --porcelain 2>$null
    if ($dirty) { Write-Warn "You have uncommitted changes; they WILL be included in the build." }
} catch { }
Write-Info "Git commit: $GitSha"

Write-Info "Checking SSH connection to $Remote ..."
& ssh @CommonSshOpts $Remote "echo connected to `$(hostname)"
if ($LASTEXITCODE -ne 0) {
    Stop-Deploy "Cannot SSH to $Remote. Check the instance is running and the security group allows SSH from your current IP."
}
Write-Ok "SSH works"

# ---------- 2. build ----------
$ImageTags = @($Images | ForEach-Object { $_.Tag })

if ($SkipBuild) {
    Write-Step "Build skipped (-SkipBuild); using existing local images"
    foreach ($tag in $ImageTags) {
        if (-not (Test-Native "docker" @("image", "inspect", $tag))) { Stop-Deploy "Image '$tag' does not exist locally. Run without -SkipBuild." }
        Write-Ok "found $tag"
    }
} else {
    foreach ($img in $Images) {
        $label = $img.Tag
        if ($img.Target) { $label = "$label (stage '$($img.Target)')" }
        Write-Step "Building image $label for $Platform"

        $buildArgs = @("build", "--platform", $Platform, "--progress=plain",
                       "--label", "org.opencontainers.image.revision=$GitSha",
                       "-f", $Dockerfile, "-t", $img.Tag)
        if ($img.Target) { $buildArgs += @("--target", $img.Target) }
        $buildArgs += "."

        Invoke-Native -Exe "docker" -ArgList $buildArgs -What "build of $($img.Tag)"
    }
}

# ---------- 3. save ----------
Write-Step "Saving image(s) to $TarPath"
if (Test-Path $TarPath) { Remove-Item $TarPath -Force }
Invoke-Native -Exe "docker" -ArgList (@("save", "-o", $TarPath) + $ImageTags) -What "docker save"
$sizeMb = (Get-Item $TarPath).Length / 1MB
Write-Info ("Archive size: {0:N1} MB" -f $sizeMb)

# ---------- 4. upload ----------
Write-Step "Preparing folders on the server"
# 700 so the uploaded env file is never readable by other users, even briefly.
Invoke-Native -Exe "ssh" -ArgList ($CommonSshOpts + @($Remote, "mkdir -p $RemoteWorkDir && chmod 700 $RemoteWorkDir")) -What "remote mkdir"

# Server-side script. Written with LF line endings so bash can run it.
$remoteScript = @'
set -euo pipefail
step() { printf '\n--> [server %s] %s\n' "$(date +%H:%M:%S)" "$1"; }

cd "__REMOTE_DIR__"

step "Installing the env file as deploy/.env (mode 600)"
# Compose reads .env from the compose file's folder, so it must sit in deploy/.
install -D -m 600 "__WORK_DIR__/app.env" deploy/.env
rm -f "__WORK_DIR__/app.env"

step "Making sure the shared network appointment-edge exists"
# Caddy joins it to reach the web client, which deploys separately.
docker network inspect appointment-edge >/dev/null 2>&1 || docker network create appointment-edge

step "Loading images"
docker load -i "__WORK_DIR__/__TAR__"

step "Starting / updating containers"
docker compose -f "__COMPOSE__" up -d --no-build --remove-orphans

step "Container status"
docker compose -f "__COMPOSE__" ps

step "Last 20 log lines per service"
docker compose -f "__COMPOSE__" logs --tail=20 --no-color

step "Cleaning up"
rm -f "__WORK_DIR__/__TAR__"
docker image prune -f
echo
echo "    Disk:"; df -h / | tail -1
echo "    Memory:"; free -h | sed -n '2,3p'
'@
$remoteScript = $remoteScript.Replace("__REMOTE_DIR__", $RemoteDir).
                              Replace("__WORK_DIR__", $RemoteWorkDir).
                              Replace("__TAR__", $TarName).
                              Replace("__COMPOSE__", $ComposeFile)
$remoteScript = $remoteScript -replace "`r", ""
$localRemoteScript = Join-Path $env:TEMP "remote-deploy.sh"
[IO.File]::WriteAllText($localRemoteScript, $remoteScript, (New-Object Text.UTF8Encoding($false)))

# LF copy of the env file: a trailing CR would end up inside every value.
$localEnvCopy = Join-Path $env:TEMP "app.env"
$envText = [IO.File]::ReadAllText((Resolve-Path $EnvFile)) -replace "`r", ""
[IO.File]::WriteAllText($localEnvCopy, $envText, (New-Object Text.UTF8Encoding($false)))

Write-Step ("Uploading images ({0:N1} MB), env file and server script" -f $sizeMb)
try {
    Invoke-Native -Exe "scp" -ArgList ($CommonSshOpts + @($TarPath, $localRemoteScript, $localEnvCopy, "${Remote}:$RemoteWorkDir/")) -What "upload"
} finally {
    Remove-Item $localEnvCopy -Force -ErrorAction SilentlyContinue
}

# ---------- 5. config sync ----------
if ($SkipConfigSync) {
    Write-Step "Config sync skipped (-SkipConfigSync)"
} else {
    Write-Step "Syncing '$LocalDeployDir' (compose file, Caddyfile) and 'migrations' to the server"
    Invoke-Native -Exe "scp" -ArgList ($CommonSshOpts + @("-r", $LocalDeployDir, "migrations", "${Remote}:$RemoteDir/")) -What "config sync"
}

# ---------- 6. load and run on the server ----------
Write-Step "Loading images and starting containers on the server"
Invoke-Native -Exe "ssh" -ArgList ($CommonSshOpts + @($Remote, "bash $RemoteWorkDir/remote-deploy.sh")) -What "remote deploy"

Remove-Item $TarPath -Force -ErrorAction SilentlyContinue

# ---------- 7. health check ----------
if ($HealthUrl) {
    Write-Step "Health check: $HealthUrl"
    $healthy = $false
    for ($i = 1; $i -le 12; $i++) {
        try {
            $resp = Invoke-WebRequest -Uri $HealthUrl -UseBasicParsing -TimeoutSec 10
            if ($resp.StatusCode -eq 200) {
                Write-Ok "HTTP 200 on attempt $i"
                Write-Info $resp.Content
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
        Stop-Deploy "Health check did not pass after 12 attempts. Check logs: ssh -i $KeyPath $Remote 'cd $RemoteDir && docker compose -f $ComposeFile logs --tail=100'"
    }
}

# ---------- summary ----------
$total = (Get-Date) - $script:Started
Write-Host ""
Write-Host ("DEPLOY SUCCEEDED in {0:N0}s  (commit {1})" -f $total.TotalSeconds, $GitSha) -ForegroundColor Green
Write-Host "Images: $($ImageTags -join ', ')" -ForegroundColor Green
Write-Host "Logs:   ssh -i $KeyPath $Remote `"cd $RemoteDir && docker compose -f $ComposeFile logs -f`"" -ForegroundColor Gray
param([string]$GoImage = 'golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $projectRoot
try {
    New-Item -ItemType Directory -Path docs/evidence -Force | Out-Null
    docker compose --progress plain build --build-arg "GO_IMAGE=$GoImage" 2>&1 | Tee-Object docs/evidence/stage6-production-build.txt
    if ($LASTEXITCODE -ne 0) { throw 'production build/race checks failed' }
    $previousDockerTest = $env:FP_DOCKER_TEST
    try {
        $env:FP_DOCKER_TEST = '1'
        go test -count=1 -v -timeout 180s ./integration -run '^TestCompose$' 2>&1 | Tee-Object docs/evidence/stage6-production-compose.txt
        if ($LASTEXITCODE -ne 0) { throw 'production Compose checks failed' }
    } finally { $env:FP_DOCKER_TEST = $previousDockerTest }
    docker image inspect banner-fingerprint-server:local banner-fingerprint-client:local --format '{{.Id}} {{.Size}} {{json .Config.User}}' | Set-Content docs/evidence/stage6-production-images.txt
    if ($LASTEXITCODE -ne 0) { throw 'production image inspection failed' }
} finally { Pop-Location }

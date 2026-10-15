$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $projectRoot
try {
    $runtimeDir = Join-Path $projectRoot 'artifacts/runtime-check'
    New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null
    $previousOS = $env:GOOS
    $previousArch = $env:GOARCH
    $previousCGO = $env:CGO_ENABLED
    try {
        $env:GOOS = 'linux'
        $env:GOARCH = 'amd64'
        $env:CGO_ENABLED = '0'
        go build -trimpath -ldflags '-s -w' -o "$runtimeDir/server" ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw 'server cross compilation failed' }
        go build -trimpath -ldflags '-s -w' -o "$runtimeDir/client" ./cmd/client
        if ($LASTEXITCODE -ne 0) { throw 'client cross compilation failed' }
    } finally {
        $env:GOOS = $previousOS
        $env:GOARCH = $previousArch
        $env:CGO_ENABLED = $previousCGO
    }
    Copy-Item -LiteralPath 'rules/fingerprints.json' -Destination "$runtimeDir/fingerprints.json"
    $dockerfile = Get-Content -LiteralPath 'Dockerfile' -Raw
    $runtime = $dockerfile.Substring($dockerfile.IndexOf('FROM scratch AS server'))
    $runtime = $runtime.Replace('COPY --from=build /out/server /server', 'COPY server /server')
    $runtime = $runtime.Replace('COPY --from=build /out/client /client', 'COPY client /client')
    $runtime = $runtime.Replace('COPY --from=build /src/rules/fingerprints.json /rules/fingerprints.json', 'COPY fingerprints.json /rules/fingerprints.json')
    [IO.File]::WriteAllText("$runtimeDir/Dockerfile", $runtime)
    docker build --network none --target server -t banner-fingerprint-server:runtime-check "$runtimeDir"
    if ($LASTEXITCODE -ne 0) { throw 'runtime server image failed' }
    docker build --network none --target client -t banner-fingerprint-client:runtime-check "$runtimeDir"
    if ($LASTEXITCODE -ne 0) { throw 'runtime client image failed' }
    $previousServerImage = $env:SERVER_IMAGE
    $previousClientImage = $env:CLIENT_IMAGE
    $previousDockerTest = $env:FP_DOCKER_TEST
    try {
        $env:SERVER_IMAGE = 'banner-fingerprint-server:runtime-check'
        $env:CLIENT_IMAGE = 'banner-fingerprint-client:runtime-check'
        $env:FP_DOCKER_TEST = '1'
        go test -count=1 -v -timeout 180s ./integration -run '^TestCompose$'
        if ($LASTEXITCODE -ne 0) { throw 'runtime Compose acceptance failed' }
    } finally {
        $env:SERVER_IMAGE = $previousServerImage
        $env:CLIENT_IMAGE = $previousClientImage
        $env:FP_DOCKER_TEST = $previousDockerTest
    }
} finally { Pop-Location }

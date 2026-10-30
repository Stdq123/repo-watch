$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    go build -ldflags '-H=windowsgui' -o RepoWatch.exe .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

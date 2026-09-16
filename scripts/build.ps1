[CmdletBinding()]
param(
    [string]$Output = "ipgw.exe",
    [string]$Version,
    [string]$SourceRepo = "ze-mu-zhou/SFR-ipgw",
    [switch]$Release,
    [string]$ReleaseRepo
)
$ErrorActionPreference = 'Stop'
$projectDir = Split-Path -Parent $PSScriptRoot
function Read-GitValue([string[]]$GitArguments) {
    # Windows PowerShell treats native stderr as an error record, even when
    # redirected; a missing tag must remain an ordinary local build.
    $ErrorActionPreference = 'SilentlyContinue'
    $value = (& git @GitArguments 2>$null | Out-String).Trim()
    return @{ Value = $value; Success = ($LASTEXITCODE -eq 0) }
}
Push-Location $projectDir
try {
    $commitResult = Read-GitValue @('rev-parse','--short','HEAD')
    $commit = $commitResult.Value
    if (!$commitResult.Success -or !$commit) { $commit = 'unknown' }
    $statusResult = Read-GitValue @('status','--porcelain','--untracked-files=normal')
    $dirty = !$statusResult.Success -or $statusResult.Value.Length -gt 0
    $tagResult = Read-GitValue @('describe','--tags','--exact-match','HEAD')
    $tag = $tagResult.Value
    if (!$tagResult.Success) { $tag = '' }
    if (!$Version) { if ($tag) { $Version = $tag } else { $Version = 'dev' } }
    $kind = 'local'
    $enabled = 'false'
    if ($Release) {
        if ($dirty -or !$tag -or $Version -ne $tag -or $Version -notmatch '^v?\d+\.\d+\.\d+$') {
            throw 'Release builds require a clean checkout at a stable semantic-version tag.'
        }
        if ($ReleaseRepo -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') {
            throw 'Release builds require -ReleaseRepo owner/repository explicitly identifying a compatible release source.'
        }
        $kind = 'release'
        $enabled = 'true'
    } else {
        $Version += '+local.' + $commit
        if ($dirty) { $Version += '.dirty'; $kind = 'local-dirty' }
        $ReleaseRepo = ''
    }
    foreach ($value in @($Version,$SourceRepo,$ReleaseRepo)) {
        if ($value -match '[\s"''`$]') { throw 'Build metadata may not contain whitespace, quotes, backticks or dollar signs.' }
    }
    $buildTime = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
    $metadata = @{
        Version=$Version; Build=$buildTime; Commit=$commit; BuildKind=$kind
        Repo=$SourceRepo; ReleaseRepo=$ReleaseRepo; UpdateEnabled=$enabled
    }
    $flags = '-s -w'
    foreach ($key in $metadata.Keys) { $flags += " -X github.com/ze-mu-zhou/SFR-ipgw.$key=$($metadata[$key])" }
    & go build -trimpath -ldflags $flags -o $Output ./cmd/ipgw
    if ($LASTEXITCODE -ne 0) { throw "go build failed ($LASTEXITCODE)" }
    Write-Output "Built $Output ($Version, $kind)"
} finally { Pop-Location }

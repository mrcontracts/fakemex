[CmdletBinding()]
param(
    [Parameter()]
    [string]$Distro = "",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$ProjectPath = "~/FakeMex"
)

$ErrorActionPreference = "Stop"

$wslCommand = Get-Command "wsl.exe" -ErrorAction SilentlyContinue
if ($null -eq $wslCommand) {
    Write-Error "wsl.exe was not found. Install WSL 2 and a Linux distribution first."
    exit 1
}

$installedDistros = @(
    & $wslCommand.Source --list --quiet 2>$null |
        ForEach-Object { $_.Replace([char]0, "").Trim() } |
        Where-Object { $_ }
)
if ($LASTEXITCODE -ne 0 -or $installedDistros.Count -eq 0) {
    Write-Error "No WSL distribution is available. Install one with: wsl.exe --install"
    exit 1
}

if ($Distro -and $installedDistros -notcontains $Distro) {
    Write-Error "WSL distribution '$Distro' is not installed. Available: $($installedDistros -join ', ')"
    exit 1
}

$launchCommand = @'
project_path=$1
case "$project_path" in
  "~") project_path=$HOME ;;
  "~/"*) project_path="$HOME/${project_path#\~/}" ;;
esac

if [[ ! -d "$project_path" ]]; then
  printf 'start-wsl.ps1: project directory not found: %s\n' "$project_path" >&2
  exit 2
fi
if [[ ! -f "$project_path/launch.sh" ]]; then
  printf 'start-wsl.ps1: launch.sh not found in: %s\n' "$project_path" >&2
  exit 2
fi

cd -- "$project_path"
exec ./launch.sh
'@
$launchCommand = $launchCommand.Replace("`r`n", "`n")

$wslArguments = @()
if ($Distro) {
    $wslArguments += @("--distribution", $Distro)
}
$wslArguments += @("--exec", "bash", "-lc", $launchCommand, "fakemex-wsl", $ProjectPath)

& $wslCommand.Source @wslArguments
exit $LASTEXITCODE

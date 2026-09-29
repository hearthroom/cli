# Install the hearthroom CLI on Windows.
#
#   irm https://raw.githubusercontent.com/hearthroom/cli/main/install.ps1 | iex
#
# Environment:
#   HEARTHROOM_VERSION      release tag to install (default: latest), e.g. v0.1.0
#   HEARTHROOM_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\Programs\hearthroom)
#
# Downloads the release archive for this machine, verifies it against
# checksums.txt from the same release, clears the download mark so SmartScreen
# does not prompt, and adds the install directory to the user PATH.
$ErrorActionPreference = 'Stop'

$Repo = 'hearthroom/cli'
$Bin = 'hearthroom'

$arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }

$version = $env:HEARTHROOM_VERSION
if (-not $version) {
  $resp = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -MaximumRedirection 0 -ErrorAction SilentlyContinue
  $location = $resp.Headers.Location
  if (-not $location) { $location = $resp.BaseResponse.ResponseUri.AbsoluteUri }
  $version = ($location -split '/tag/')[-1]
}
if (-not $version) { throw 'could not determine the latest release; set HEARTHROOM_VERSION' }
$plain = $version.TrimStart('v')

$asset = "${Bin}_${plain}_windows_${arch}.zip"
$base = "https://github.com/$Repo/releases/download/$version"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("hearthroom-" + [System.Guid]::NewGuid().ToString('n'))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Host "Downloading $Bin $version for windows/$arch..."
  Invoke-WebRequest -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
  Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')

  $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -like "*  $asset" }
  if (-not $line) { throw "no checksum for $asset in checksums.txt" }
  $expected = ($line -split '\s+')[0].ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash.ToLowerInvariant()
  if ($expected -ne $actual) { throw "checksum mismatch for $asset" }

  Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath (Join-Path $tmp 'x') -Force

  $dir = $env:HEARTHROOM_INSTALL_DIR
  if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\hearthroom' }
  New-Item -ItemType Directory -Path $dir -Force | Out-Null
  $target = Join-Path $dir "$Bin.exe"
  Copy-Item (Join-Path $tmp "x\$Bin.exe") $target -Force
  Unblock-File -Path $target -ErrorAction SilentlyContinue

  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (-not (($userPath -split ';') -contains $dir)) {
    [Environment]::SetEnvironmentVariable('Path', "$dir;$userPath", 'User')
    $env:Path = "$dir;$env:Path"
    Write-Host "Added $dir to your user PATH (open a new terminal to pick it up)."
  }

  Write-Host "Installed $target"
  & $target version
  Write-Host ""
  Write-Host "Tab completion: add this line to your PowerShell profile (notepad `$PROFILE):"
  Write-Host "  hearthroom completion powershell | Out-String | Invoke-Expression"
}
finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

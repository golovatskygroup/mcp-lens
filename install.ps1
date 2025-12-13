# install.ps1
# Downloads a mcp-lens release asset for the current Windows arch, verifies it against checksums.txt,
# installs the binary into %LOCALAPPDATA%\mcp-lens\bin, and prints the installed absolute path.

$Repo = "golovatskygroup/mcp-lens"
$Version = $env:MCP_LENS_VERSION
if (-not $Version) { $Version = "v1.0.2" }

$InstallDir = $env:MCP_LENS_INSTALL_DIR
if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA "mcp-lens\bin" }

$Arch = $env:PROCESSOR_ARCHITECTURE
if ($Arch -eq "ARM64") { $Arch = "arm64" } else { $Arch = "amd64" }

$Asset = ("mcp-lens_{0}_windows_{1}.zip" -f $Version.TrimStart('v'), $Arch)
$BaseUrl = "https://github.com/$Repo/releases/download/$Version"

$Work = Join-Path $env:TEMP ("mcp-lens-" + $Version + "-" + [guid]::NewGuid().ToString())
New-Item -ItemType Directory -Force -Path $Work | Out-Null

try {
  Invoke-WebRequest -Uri ("$BaseUrl/checksums.txt") -OutFile (Join-Path $Work "checksums.txt")
  Invoke-WebRequest -Uri ("$BaseUrl/$Asset") -OutFile (Join-Path $Work $Asset)

  $Line = Select-String -Path (Join-Path $Work "checksums.txt") -Pattern ("\s" + [regex]::Escape($Asset) + "$") | Select-Object -First 1
  if (-not $Line) { throw "Checksum entry not found for $Asset" }

  $Expected = ($Line.Line -split "\s+")[0].ToLower()
  $Got = (Get-FileHash (Join-Path $Work $Asset) -Algorithm SHA256).Hash.ToLower()
  if ($Expected -ne $Got) { throw "Checksum mismatch for $Asset (expected=$Expected got=$Got)" }

  New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
  Expand-Archive -Force (Join-Path $Work $Asset) $Work

  $Src = Join-Path $Work "mcp-lens.exe"
  $Dst = Join-Path $InstallDir "mcp-lens.exe"
  Move-Item -Force $Src $Dst

  Write-Output (Resolve-Path $Dst).Path
}
finally {
  Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue
}

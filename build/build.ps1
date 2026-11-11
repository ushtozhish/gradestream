# Сборка GradeStream на Windows: программа + установщик.
# Нужно: Go 1.27+ (https://go.dev/dl/), PowerShell 5+, доступ в интернет.
# Запуск из корня репозитория:  powershell -ExecutionPolicy Bypass -File build\build.ps1
$ErrorActionPreference = 'Stop'
$Version     = (Select-String -Path app\main.go -Pattern 'const appVersion = "([^"]+)"').Matches[0].Groups[1].Value
$Cloudflared = '2026.10.7'
$Payload     = 'installer\payload'
New-Item -ItemType Directory -Force -Path $Payload, dist | Out-Null

# 1. Официальный cloudflared с проверкой цифровой подписи Cloudflare
$cf = "$Payload\cloudflared.exe"
if (-not (Test-Path $cf)) {
  Write-Host "Скачиваю cloudflared $Cloudflared..."
  Invoke-WebRequest "https://github.com/cloudflare/cloudflared/releases/download/$Cloudflared/cloudflared-windows-amd64.exe" -OutFile $cf
}
$sig = Get-AuthenticodeSignature $cf
if ($sig.Status -ne 'Valid' -or $sig.SignerCertificate.Subject -notmatch 'O="?Cloudflare, Inc\.') {
  Remove-Item $cf; throw "cloudflared.exe: подпись Cloudflare не подтверждена ($($sig.Status))"
}
Copy-Item docs\gradestream.ico "$Payload\gradestream.ico" -Force

# 2. Программа
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
Push-Location app
go build -mod=mod -trimpath -ldflags "-s -w -H windowsgui" -o "..\$Payload\GradeStream.exe" .
Pop-Location

# 3. Установщик (вшивает программу, cloudflared и иконку)
Push-Location installer
go build -trimpath -ldflags "-s -w -H windowsgui" -o "..\dist\GradeStream-Setup-$Version.exe" .
Pop-Location
Write-Host "Готово: dist\GradeStream-Setup-$Version.exe"

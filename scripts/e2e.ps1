$ErrorActionPreference = "Stop"
$base = if ($env:VISIONAI_BASE_URL) { $env:VISIONAI_BASE_URL.TrimEnd("/") } else { "http://localhost:58080" }
$health = Invoke-RestMethod "$base/health"
if (-not $health) { throw "Backend health check returned no data." }
$loginBody = @{ username = "admin"; password = "admin123" } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "$base/admin-api/system/auth/login" -Headers @{ "tenant-id" = "1" } -ContentType "application/json" -Body $loginBody
if (-not $login.data.accessToken) { throw "Login did not return an access token." }
$headers = @{ Authorization = "Bearer $($login.data.accessToken)" }
$checks = @(
    "/admin-api/ai-platform/dashboard/summary",
    "/admin-api/ai-platform/projects?pageNo=1&pageSize=1",
    "/admin-api/ai-platform/jobs?pageNo=1&pageSize=1",
    "/admin-api/ai-platform/resources/overview",
    "/admin-api/ai-platform/integrations?pageNo=1&pageSize=1",
    "/admin-api/ai-platform/audit-events?pageNo=1&pageSize=1"
)
foreach ($path in $checks) {
    Invoke-RestMethod -Headers $headers -Uri "$base$path" | Out-Null
}
Write-Host "VisionAI smoke: health, auth and core read surfaces passed."

$ErrorActionPreference = "Stop"
$base = if ($env:VISIONAI_BASE_URL) { $env:VISIONAI_BASE_URL.TrimEnd("/") } else { "http://localhost:58080" }
$workbenchOrigin = if ($env:VISIONAI_WORKBENCH_ORIGIN) { $env:VISIONAI_WORKBENCH_ORIGIN.TrimEnd("/") } else { "http://localhost:48080" }
$cvatUI = if ($env:VISIONAI_CVAT_UI_URL) { $env:VISIONAI_CVAT_UI_URL.TrimEnd("/") } else { "http://localhost:28080" }
$fiftyOneUI = if ($env:VISIONAI_FIFTYONE_UI_URL) { $env:VISIONAI_FIFTYONE_UI_URL.TrimEnd("/") } else { "http://localhost:25151" }
$fiftyOneAPI = if ($env:VISIONAI_FIFTYONE_API_URL) { $env:VISIONAI_FIFTYONE_API_URL.TrimEnd("/") } else { "http://localhost:25152" }
$health = Invoke-RestMethod "$base/health"
if (-not $health) { throw "Backend health check returned no data." }
$fiftyOneHealth = Invoke-RestMethod "$fiftyOneAPI/health"
if ($fiftyOneHealth.status -ne "UP" -or $fiftyOneHealth.uiStatus -ne "UP") {
    throw "FiftyOne bridge or UI is unavailable."
}
$fiftyOnePage = Invoke-WebRequest -UseBasicParsing "$fiftyOneUI/"
if ($fiftyOnePage.StatusCode -ne 200) { throw "FiftyOne UI did not return HTTP 200." }
$cvatPage = Invoke-WebRequest -UseBasicParsing "$cvatUI/"
if ($cvatPage.StatusCode -ne 200) { throw "CVAT UI did not return HTTP 200." }
if ($cvatPage.Headers["X-Frame-Options"]) {
    throw "CVAT UI still sends X-Frame-Options and cannot be embedded."
}
$cvatCsp = [string]$cvatPage.Headers["Content-Security-Policy"]
if ($cvatCsp -notmatch "frame-ancestors" -or $cvatCsp -notmatch [regex]::Escape($workbenchOrigin)) {
    throw "CVAT UI does not restrict iframe embedding to the VisionAI workbench."
}
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
Write-Host "VisionAI smoke: health, embedded CVAT/FiftyOne, auth and core read surfaces passed."

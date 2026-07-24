#!/usr/bin/env sh
set -eu
base="${VISIONAI_BASE_URL:-http://localhost:58080}"
workbench_origin="${VISIONAI_WORKBENCH_ORIGIN:-http://localhost:48080}"
cvat_ui="${VISIONAI_CVAT_UI_URL:-http://localhost:28080}"
fiftyone_ui="${VISIONAI_FIFTYONE_UI_URL:-http://localhost:25151}"
fiftyone_api="${VISIONAI_FIFTYONE_API_URL:-http://localhost:25152}"
curl -fsS "$base/health" >/dev/null
curl -fsS "$fiftyone_ui/" >/dev/null
cvat_headers="$(curl -fsSI "$cvat_ui/")"
if printf '%s\n' "$cvat_headers" | grep -qi '^x-frame-options:'; then
  echo "CVAT UI still sends X-Frame-Options and cannot be embedded." >&2
  exit 1
fi
printf '%s\n' "$cvat_headers" | grep -qi '^content-security-policy:.*frame-ancestors'
printf '%s\n' "$cvat_headers" | grep -Fqi "$workbench_origin"
fiftyone_health="$(curl -fsS "$fiftyone_api/health")"
printf '%s' "$fiftyone_health" | jq -e '.status == "UP" and .uiStatus == "UP"' >/dev/null
login="$(curl -fsS -H 'Content-Type: application/json' -H 'tenant-id: 1' -d '{"username":"admin","password":"admin123"}' "$base/admin-api/system/auth/login")"
token="$(printf '%s' "$login" | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')"
[ -n "$token" ]
for path in \
  dashboard/summary \
  "projects?pageNo=1&pageSize=1" \
  "jobs?pageNo=1&pageSize=1" \
  resources/overview \
  "integrations?pageNo=1&pageSize=1" \
  "audit-events?pageNo=1&pageSize=1"
do
  curl -fsS -H "Authorization: Bearer $token" "$base/admin-api/ai-platform/$path" >/dev/null
done
echo "VisionAI smoke: health, embedded CVAT/FiftyOne, auth and core read surfaces passed."

#!/usr/bin/env sh
set -eu
base="${VISIONAI_BASE_URL:-http://localhost:58080}"
curl -fsS "$base/health" >/dev/null
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
echo "VisionAI smoke: health, auth and core read surfaces passed."

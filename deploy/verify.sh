#!/usr/bin/env bash
# End-to-end smoke test against a running API.
#
#   deploy/verify.sh [BASE_URL]      default http://localhost:8080
#
# Every expected number below was measured from the real KMZ export before any of
# this code existed, so a mismatch means the refactor changed behaviour.
#
# Requires: curl, jq. Admin checks need ADMIN_EMAIL and ADMIN_PASSWORD.
set -uo pipefail

BASE="${1:-http://localhost:8080}"
API="$BASE/api/v1"
XRW='X-Requested-With: pm-admin'

pass=0 fail=0
ok()   { printf '  \033[32mok\033[0m   %s\n' "$1"; pass=$((pass+1)); }
bad()  { printf '  \033[31mFAIL\033[0m %s\n       expected: %s\n       actual:   %s\n' "$1" "$2" "$3"; fail=$((fail+1)); }
check(){ # description expected actual
  if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "$2" "$3"; fi
}

echo "verifying $BASE"
echo
echo "public endpoints"

check "health responds ok" "ok" \
  "$(curl -sf "$API/health" | jq -r '.status // "ERROR"')"

check "catalog lists 4 areas" "4" \
  "$(curl -sf "$API/catalog" | jq '.areas | length')"

# Six, not five: the dialog dropdown offers every category including the unused
# "Other" fallback, while the UI renders chips only where facilityCount > 0.
check "catalog lists 6 categories" "6" \
  "$(curl -sf "$API/catalog" | jq '.categories | length')"

check "exactly 5 categories are in use" "5" \
  "$(curl -sf "$API/catalog" | jq '[.categories[] | select(.facilityCount > 0)] | length')"

check "Other is the unused fallback" "true" \
  "$(curl -sf "$API/catalog" | jq '.categories[] | select(.key=="Other") | .isFallback and .facilityCount == 0')"

check "area short codes" "Sedayu JGC KHI MTL" \
  "$(curl -sf "$API/catalog" | jq -r '[.areas[].shortCode] | join(" ")')"

check "43 facilities total" "43" \
  "$(curl -sf "$API/facilities" | jq '.total')"

check "KHI has 27" "27" \
  "$(curl -sf "$API/facilities?area=KHI" | jq '.total')"
check "Sedayu has 6" "6" \
  "$(curl -sf "$API/facilities?area=Sedayu" | jq '.total')"
check "JGC has 6" "6" \
  "$(curl -sf "$API/facilities?area=JGC" | jq '.total')"
check "Metland has 4" "4" \
  "$(curl -sf "$API/facilities?area=Metland" | jq '.total')"

check "17 schools" "17" \
  "$(curl -sf "$API/facilities?category=School" | jq '.total')"
check "schools + hospitals = 22" "22" \
  "$(curl -sf "$API/facilities?category=School&category=Hospital" | jq '.total')"

check "search 'shell' finds 1" "1" \
  "$(curl -sf "$API/facilities?q=shell" | jq '.total')"

# The two hand-corrected records from the source export.
check "SIS JGC is a School, not a Gas Station" "JGC School" \
  "$(curl -sf "$API/facilities?q=SIS%20JGC" | jq -r '.items[0] | "\(.areaKey) \(.categoryKey)"')"
check "Suzuki was renamed and placed in Sedayu" "Suzuki Sedayu Sedayu" \
  "$(curl -sf "$API/facilities?q=Suzuki" | jq -r '.items[0] | "\(.name) \(.areaKey)"')"

check "4 boundaries" "4" \
  "$(curl -sf "$API/boundaries" | jq '.items | length')"
check "boundaries are GeoJSON MultiPolygons" "MultiPolygon" \
  "$(curl -sf "$API/boundaries" | jq -r '.items[0].geometry.type')"

check "/map bundles everything in one call" "4 6 43 4" \
  "$(curl -sf "$API/map" | jq -r '"\(.areas|length) \(.categories|length) \(.facilities|length) \(.boundaries|length)"')"

echo
echo "auth boundary"

check "anonymous POST /pins is 401" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/pins" -H "$XRW" -H 'Content-Type: application/json' -d '{}')"
check "anonymous PATCH /pins is 401" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X PATCH "$API/pins/00000000-0000-0000-0000-000000000000" -H "$XRW" -H 'Content-Type: application/json' -d '{}')"
check "anonymous DELETE /pins is 401" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$API/pins/00000000-0000-0000-0000-000000000000" -H "$XRW")"
check "anonymous POST /import is 401" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/import" -H "$XRW")"
check "anonymous GET /auth/me is 401" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' "$API/auth/me")"

# CSRF: a cross-site form POST cannot set a custom header, so its absence is
# rejected before authentication is even considered.
check "mutation without X-Requested-With is 403" "403" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/pins" -H 'Content-Type: application/json' -d '{}')"

check "bad credentials are rejected" "401" \
  "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$API/auth/login" -H "$XRW" -H 'Content-Type: application/json' -d '{"email":"nobody@example.com","password":"wrong-password-here"}')"

if [ -z "${ADMIN_EMAIL:-}" ] || [ -z "${ADMIN_PASSWORD:-}" ]; then
  echo
  echo "  (skipping admin checks — set ADMIN_EMAIL and ADMIN_PASSWORD to run them)"
else
  echo
  echo "admin round trip"
  JAR="$(mktemp)"
  trap 'rm -f "$JAR"' EXIT

  check "login sets a session" "204" \
    "$(curl -s -o /dev/null -w '%{http_code}' -c "$JAR" -X POST "$API/auth/login" \
        -H "$XRW" -H 'Content-Type: application/json' \
        -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}")"

  check "session cookie is HttpOnly" "TRUE" \
    "$(awk '/pm_session/ {print toupper($0) ~ /#HTTPONLY/ ? "TRUE" : "FALSE"}' "$JAR" | head -1)"

  check "/auth/me returns the admin" "$ADMIN_EMAIL" \
    "$(curl -sf -b "$JAR" "$API/auth/me" | jq -r '.email')"

  PIN=$(curl -sf -b "$JAR" -X POST "$API/pins" -H "$XRW" -H 'Content-Type: application/json' \
    -d '{"name":"Verify Pin","areaKey":"JGC","categoryKey":"Showroom Dealer","lat":-6.17,"lng":106.96}')
  PIN_ID=$(echo "$PIN" | jq -r '.id')
  check "created pin is manual" "manual" "$(echo "$PIN" | jq -r '.source')"
  check "facility count rose to 44" "44" "$(curl -sf "$API/facilities" | jq '.total')"

  check "pin can be renamed" "Verify Pin Renamed" \
    "$(curl -sf -b "$JAR" -X PATCH "$API/pins/$PIN_ID" -H "$XRW" -H 'Content-Type: application/json' \
        -d '{"name":"Verify Pin Renamed"}' | jq -r '.name')"

  # Imported facilities are not editable: the update query carries
  # `AND source = 'manual'`, so targeting one is indistinguishable from a miss.
  IMPORTED=$(curl -sf "$API/facilities?q=Shell" | jq -r '.items[0].id')
  check "imported facility is not editable" "404" \
    "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR" -X PATCH "$API/pins/$IMPORTED" \
        -H "$XRW" -H 'Content-Type: application/json' -d '{"name":"nope"}')"

  KMZ="$(dirname "$0")/../data/facility-mapping.kmz"
  if [ -f "$KMZ" ]; then
    REIMPORT=$(curl -sf -b "$JAR" -X POST "$API/import" -H "$XRW" -F "file=@$KMZ")
    # The crown jewel: re-importing changes nothing and leaves the manual pin be.
    check "re-import reports 43 unchanged" "0 0 43 0" \
      "$(echo "$REIMPORT" | jq -r '.features | "\(.inserted) \(.updated) \(.unchanged) \(.deleted)"')"
    check "re-import drops nothing" "0" \
      "$(echo "$REIMPORT" | jq '.dropped | length')"
    check "manual pin survived the re-import" "44" \
      "$(curl -sf "$API/facilities" | jq '.total')"
  fi

  check "pin can be deleted" "204" \
    "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR" -X DELETE "$API/pins/$PIN_ID" -H "$XRW")"
  check "back to 43 facilities" "43" "$(curl -sf "$API/facilities" | jq '.total')"

  check "logout clears the session" "401" \
    "$(curl -s -o /dev/null -w '%{http_code}' -b "$JAR" -c "$JAR" -X POST "$API/auth/logout" -H "$XRW" >/dev/null;
       curl -s -o /dev/null -w '%{http_code}' -b "$JAR" "$API/auth/me")"
fi

echo
printf '%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

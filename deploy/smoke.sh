#!/usr/bin/env bash
# 배포 스모크: 떠 있는 API 에 실제 요청을 보내 기대한 상태코드와 서버 버전을 확인한다.
#   bash deploy/smoke.sh http://localhost:8081                 # 공개·보호(401) 경로만
#   bash deploy/smoke.sh http://localhost:8082 <devtoken JWT>  # 보호 경로·경로 탐색까지(개발 스택)
# 상태코드가 하나라도 다르면 종료코드 1. 서버 /health 의 version 이 현재 체크아웃 커밋과 다르면 실패(다른 트리에서 빌드한 이미지).
set -u
BASE="${1:?사용법: smoke.sh <base-url> [devtoken]}"
TOKEN="${2:-}"
fail=0

check() { # 이름 기대코드 curl인자...
  local name="$1" want="$2"; shift 2
  local got
  got=$(curl -s -o /dev/null -w '%{http_code}' -m 30 "$@")
  if [ "$got" = "$want" ]; then
    printf 'ok    %-44s %s\n' "$name" "$got"
  else
    printf 'FAIL  %-44s got %s want %s\n' "$name" "$got" "$want"
    fail=1
  fi
}

echo "== $BASE"
health=$(curl -s -m 30 "$BASE/health")
if ! echo "$health" | grep -q '"db":"ok"' || ! echo "$health" | grep -q '"otp":"ok"'; then
  echo "FAIL  /health 본문: $health"; fail=1
else
  echo "ok    /health db·otp ok"
fi
# 서버가 자기 빌드 커밋을 말한다. 현재 체크아웃과 다르면 엉뚱한 트리에서 빌드한 이미지다.
if command -v git >/dev/null && git rev-parse --short HEAD >/dev/null 2>&1; then
  want=$(git rev-parse --short HEAD)
  got=$(echo "$health" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
  if [ "$got" = "$want" ]; then
    echo "ok    version $got = HEAD"
  else
    echo "FAIL  version: 서버 '$got' ≠ 체크아웃 '$want'"; fail=1
  fi
fi

check "GET /auth/config" 200 "$BASE/auth/config"
# GBFS 는 api 기동 뒤 poller 가 첫 스냅숏을 받기 전까지 503(gbfs snapshot not ready)이라 잠시 기다린다(실측 0.4초,
# 상류가 느리면 더 김). 20초가 지나도 200 이 아니면 진짜 실패(키 없음·상류 장애)로 FAIL.
gbfs=000
for _ in $(seq 1 20); do
  gbfs=$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$BASE/gbfs/station_status.json")
  [ "$gbfs" = 200 ] && break
  sleep 1
done
if [ "$gbfs" = 200 ]; then
  printf 'ok    %-44s %s\n' "GET /gbfs/station_status.json" 200
else
  printf 'FAIL  %-44s got %s want 200 (20초 대기 후)\n' "GET /gbfs/station_status.json" "$gbfs"; fail=1
fi
check "POST /auth/google (가짜 토큰→401)" 401 -X POST -H 'Content-Type: application/json' \
  -d '{"id_token":"bogus"}' "$BASE/auth/google"
# 보호 경로는 토큰 없이 401 이어야 한다. 404 면 그 API 가 이 빌드에 없는 것이다(2026-09-22 즐겨찾기 사고).
for p in /users/me /users/me/speed /users/me/favorites /routes/recent "/places/search?q=x" \
  "/places/reverse?lat=37.5&lon=127" "/places/landmark?lat=37.5&lon=127" /tiles/14/13977/6363.png; do
  check "GET $p (토큰 없음→401)" 401 "$BASE$p"
done
check "POST /routes/plan (토큰 없음→401)" 401 -X POST -H 'Content-Type: application/json' -d '{}' "$BASE/routes/plan"
check "POST /trips (토큰 없음→401)" 401 -X POST "$BASE/trips"

if [ -n "$TOKEN" ]; then
  H="Authorization: Bearer $TOKEN"
  echo "== 인증 경로 (devtoken)"
  check "GET /users/me" 200 -H "$H" "$BASE/users/me"
  check "GET /users/me/speed" 200 -H "$H" "$BASE/users/me/speed"
  check "GET /users/me/favorites" 200 -H "$H" "$BASE/users/me/favorites"
  check "GET /routes/recent" 200 -H "$H" "$BASE/routes/recent"
  check "GET /places/search?q=seoul" 200 -H "$H" "$BASE/places/search?q=seoul"
  check "GET /places/reverse 시청" 200 -H "$H" "$BASE/places/reverse?lat=37.5665&lon=126.978"
  check "GET /places/landmark 시청" 200 -H "$H" "$BASE/places/landmark?lat=37.5665&lon=126.978"
  check "GET /tiles/14/13977/6363.png" 200 -H "$H" "$BASE/tiles/14/13977/6363.png"
  plan='{"origin":{"lat":37.5547,"lon":126.9707,"name":"서울역"},'
  plan="$plan"'"destination":{"lat":37.4979,"lon":127.0276,"name":"강남역"}}'
  body=$(curl -s -m 90 -H "$H" -H 'Content-Type: application/json' -d "$plan" "$BASE/routes/plan")
  if echo "$body" | grep -q '"legs"'; then
    echo "ok    POST /routes/plan 서울역→강남역 (legs 있음)"
  else
    echo "FAIL  POST /routes/plan 본문에 legs 없음: ${body:0:200}"; fail=1
  fi
  busan='{"origin":{"lat":35.1,"lon":129.0,"name":"부산"},'
  busan="$busan"'"destination":{"lat":37.4979,"lon":127.0276,"name":"강남역"}}'
  check "POST /routes/plan 부산 출발(→400)" 400 -H "$H" -H 'Content-Type: application/json' \
    -d "$busan" "$BASE/routes/plan"
fi

if [ "$fail" = 0 ]; then echo "SMOKE PASS"; else echo "SMOKE FAIL"; fi
exit $fail

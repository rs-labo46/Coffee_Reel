#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(
  cd "$(dirname "${BASH_SOURCE[0]}")"
  pwd
)"

ROOT_DIR="$(
  cd "${SCRIPT_DIR}/../.."
  pwd
)"

cd "${ROOT_DIR}"

API_BASE_URL="${API_BASE_URL:-}"
RUNS="${RUNS:-100}"
WARMUP_RUNS="${WARMUP_RUNS:-5}"
SCALES="${SCALES:-100 1000 10000}"

if [[ -z "${API_BASE_URL}" ]]; then
  echo "API_BASE_URL is required."
  echo
  echo "Example:"
  echo "API_BASE_URL=http://localhost:<PORT> ./backend/scripts/benchmark_search.sh"
  exit 1
fi

API_BASE_URL="${API_BASE_URL%/}"

if ! [[ "${RUNS}" =~ ^[1-9][0-9]*$ ]]; then
  echo "RUNS must be a positive integer."
  exit 1
fi

if ! [[ "${WARMUP_RUNS}" =~ ^[0-9]+$ ]]; then
  echo "WARMUP_RUNS must be zero or a positive integer."
  exit 1
fi

command -v curl >/dev/null 2>&1 || {
  echo "curl is required."
  exit 1
}

command -v docker >/dev/null 2>&1 || {
  echo "docker is required."
  exit 1
}

docker compose version >/dev/null 2>&1 || {
  echo "docker compose is required."
  exit 1
}

for scale in ${SCALES}; do
  case "${scale}" in
    100|1000|10000)
      ;;
    *)
      echo "Unsupported scale: ${scale}"
      echo "Allowed scales: 100 1000 10000"
      exit 1
      ;;
  esac
done

echo "Checking API health..."

curl -fsS \
  "${API_BASE_URL}/health" \
  >/dev/null

TIMESTAMP="$(
  date +"%Y%m%d-%H%M%S"
)"

RESULT_DIR="${ROOT_DIR}/backend/docs/search_performance_results/${TIMESTAMP}"

mkdir -p "${RESULT_DIR}"

RAW_CSV="${RESULT_DIR}/api_raw.csv"
SUMMARY_CSV="${RESULT_DIR}/summary.csv"
ENVIRONMENT_FILE="${RESULT_DIR}/environment.txt"

echo \
"scale,case,run,dns,connect,tls,ttfb,total,status" \
> "${RAW_CSV}"

echo \
"scale,case,runs,p50_ttfb,p95_ttfb,max_ttfb,p50_total,p95_total,max_total,over_1s,over_5s,non_200" \
> "${SUMMARY_CSV}"

write_environment() {
  {
    echo "measured_at=$(date -Iseconds)"
    echo "api_base_url=${API_BASE_URL}"
    echo "runs=${RUNS}"
    echo "warmup_runs=${WARMUP_RUNS}"
    echo "scales=${SCALES}"
    echo

    echo "[git]"
    echo "commit=$(
      git rev-parse HEAD 2>/dev/null ||
        echo unknown
    )"

    echo "status:"
    git status --short 2>/dev/null || true
    echo

    echo "[system]"
    uname -a || true
    echo

    echo "[docker compose]"
    docker compose version || true
    echo

    echo "[postgresql]"
    docker compose exec -T db \
      sh -lc \
      'psql -At -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT version();"' \
      || true
  } > "${ENVIRONMENT_FILE}"
}

seed_scale() {
  local scale="$1"

  echo
  echo "========================================"
  echo "Seeding ${scale} benchmark videos"
  echo "========================================"

  docker compose run \
    --rm \
    migrate \
    go run ./cmd/seed_search_benchmark \
      --confirm-local \
      --count="${scale}"
}

verify_database_count() {
  local expected="$1"

  local actual

  actual="$(
    docker compose exec -T db \
      sh -lc \
      'psql -At -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
      <<'SQL'
SELECT COUNT(*)
FROM videos
WHERE original_object_key LIKE
  'videos/benchmark-search/%';
SQL
  )"

  actual="$(
    echo "${actual}" |
      tr -d '[:space:]'
  )"

  if [[ "${actual}" != "${expected}" ]]; then
    echo \
      "Benchmark row count mismatch: expected=${expected} actual=${actual}"
    exit 1
  fi

  echo \
    "Benchmark row count verified: ${actual}"
}

run_explain() {
  local scale="$1"
  local output_file="${RESULT_DIR}/explain_${scale}.txt"

  echo \
    "Running EXPLAIN ANALYZE for ${scale} rows..."

  docker compose exec -T db \
    sh -lc \
    'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
    > "${output_file}" \
    <<'SQL'
\echo
\echo ============================================================
\echo SEARCH INDEXES
\echo ============================================================

SELECT
  indexname,
  indexdef
FROM pg_indexes
WHERE indexname IN (
  'idx_videos_public_feed',
  'idx_videos_public_category',
  'idx_videos_public_title_trgm'
)
ORDER BY indexname;

\echo
\echo ============================================================
\echo A. PUBLIC FEED
\echo ============================================================

EXPLAIN (
  ANALYZE,
  BUFFERS,
  COSTS,
  TIMING,
  SUMMARY
)
SELECT
  videos.id
FROM videos
WHERE
  videos.processing_status = 'ready'
  AND videos.publish_status = 'published'
  AND videos.deleted_at IS NULL
ORDER BY
  videos.created_at DESC,
  videos.id DESC
LIMIT 21;

\echo
\echo ============================================================
\echo B. CATEGORY
\echo ============================================================

EXPLAIN (
  ANALYZE,
  BUFFERS,
  COSTS,
  TIMING,
  SUMMARY
)
SELECT
  videos.id
FROM videos
WHERE
  videos.processing_status = 'ready'
  AND videos.publish_status = 'published'
  AND videos.deleted_at IS NULL
  AND videos.category = 'latte_art'
ORDER BY
  videos.created_at DESC,
  videos.id DESC
LIMIT 21;

\echo
\echo ============================================================
\echo C. TITLE PARTIAL MATCH
\echo ============================================================

EXPLAIN (
  ANALYZE,
  BUFFERS,
  COSTS,
  TIMING,
  SUMMARY
)
SELECT
  videos.id
FROM videos
WHERE
  videos.processing_status = 'ready'
  AND videos.publish_status = 'published'
  AND videos.deleted_at IS NULL
  AND lower(videos.title)
      LIKE '%latte-art%'
      ESCAPE '\'
ORDER BY
  videos.created_at DESC,
  videos.id DESC
LIMIT 21;

\echo
\echo ============================================================
\echo D. TITLE + CATEGORY
\echo ============================================================

EXPLAIN (
  ANALYZE,
  BUFFERS,
  COSTS,
  TIMING,
  SUMMARY
)
SELECT
  videos.id
FROM videos
WHERE
  videos.processing_status = 'ready'
  AND videos.publish_status = 'published'
  AND videos.deleted_at IS NULL
  AND videos.category = 'latte_art'
  AND lower(videos.title)
      LIKE '%latte-art%'
      ESCAPE '\'
ORDER BY
  videos.created_at DESC,
  videos.id DESC
LIMIT 21;

\echo
\echo ============================================================
\echo E. SIMILARITY FALLBACK
\echo ============================================================

EXPLAIN (
  ANALYZE,
  BUFFERS,
  COSTS,
  TIMING,
  SUMMARY
)
SELECT
  videos.id,
  word_similarity(
    lower('latte-artt'),
    lower(videos.title)
  ) AS similarity
FROM videos
WHERE
  videos.processing_status = 'ready'
  AND videos.publish_status = 'published'
  AND videos.deleted_at IS NULL
  AND lower('latte-artt')
      <% lower(videos.title)
ORDER BY
  similarity DESC,
  videos.created_at DESC,
  videos.id DESC
LIMIT 21;
SQL
}

benchmark_case() {
  local scale="$1"
  local case_name="$2"
  local expected_result_type="$3"

  shift 3

  local response_file="${RESULT_DIR}/${scale}_${case_name}_first_response.json"

  echo \
    "  ${case_name}: validating response..."

  curl \
    -fsS \
    -G \
    --data-urlencode "limit=20" \
    "$@" \
    -o "${response_file}" \
    "${API_BASE_URL}/videos"

  if ! grep -Eq \
    "\"result_type\"[[:space:]]*:[[:space:]]*\"${expected_result_type}\"" \
    "${response_file}"; then

    echo \
      "Unexpected result_type for ${case_name}."

    echo \
      "Expected: ${expected_result_type}"

    echo \
      "Response saved to: ${response_file}"

    exit 1
  fi

  if (( WARMUP_RUNS > 0 )); then
    echo \
      "  ${case_name}: ${WARMUP_RUNS} warm-up requests..."

    local warmup

    for ((warmup = 1; warmup <= WARMUP_RUNS; warmup++)); do
      curl \
        -fsS \
        -G \
        --data-urlencode "limit=20" \
        "$@" \
        -o /dev/null \
        "${API_BASE_URL}/videos"
    done
  fi

  echo \
    "  ${case_name}: measuring ${RUNS} requests..."

  local run

  for ((run = 1; run <= RUNS; run++)); do
    local metrics
    local status

    metrics="$(
      curl \
        -sS \
        -G \
        --data-urlencode "limit=20" \
        "$@" \
        -o /dev/null \
        -w "%{time_namelookup},%{time_connect},%{time_appconnect},%{time_starttransfer},%{time_total},%{http_code}" \
        "${API_BASE_URL}/videos"
    )"

    status="${metrics##*,}"

    printf \
      "%s,%s,%d,%s\n" \
      "${scale}" \
      "${case_name}" \
      "${run}" \
      "${metrics}" \
      >> "${RAW_CSV}"

    if [[ "${status}" != "200" ]]; then
      echo \
        "HTTP ${status} detected: scale=${scale} case=${case_name} run=${run}"
      exit 1
    fi
  done
}

percentile_from_sorted_file() {
  local file="$1"
  local percentile="$2"

  local count
  local index

  count="$(
    wc -l < "${file}" |
      tr -d '[:space:]'
  )"

  if [[ "${count}" == "0" ]]; then
    echo ""
    return
  fi

  index=$(( (count * percentile + 99) / 100 ))

  if (( index < 1 )); then
    index=1
  fi

  sed -n \
    "${index}p" \
    "${file}"
}

summarize_case() {
  local scale="$1"
  local case_name="$2"

  local ttfb_file
  local total_file

  ttfb_file="$(mktemp)"
  total_file="$(mktemp)"

  awk \
    -F, \
    -v scale="${scale}" \
    -v case_name="${case_name}" \
    '
      NR > 1 &&
      $1 == scale &&
      $2 == case_name {
        print $7
      }
    ' \
    "${RAW_CSV}" |
    sort -n \
    > "${ttfb_file}"

  awk \
    -F, \
    -v scale="${scale}" \
    -v case_name="${case_name}" \
    '
      NR > 1 &&
      $1 == scale &&
      $2 == case_name {
        print $8
      }
    ' \
    "${RAW_CSV}" |
    sort -n \
    > "${total_file}"

  local count
  local p50_ttfb
  local p95_ttfb
  local max_ttfb
  local p50_total
  local p95_total
  local max_total
  local over_1s
  local over_5s
  local non_200

  count="$(
    wc -l < "${total_file}" |
      tr -d '[:space:]'
  )"

  p50_ttfb="$(
    percentile_from_sorted_file \
      "${ttfb_file}" \
      50
  )"

  p95_ttfb="$(
    percentile_from_sorted_file \
      "${ttfb_file}" \
      95
  )"

  max_ttfb="$(
    tail -n 1 \
      "${ttfb_file}"
  )"

  p50_total="$(
    percentile_from_sorted_file \
      "${total_file}" \
      50
  )"

  p95_total="$(
    percentile_from_sorted_file \
      "${total_file}" \
      95
  )"

  max_total="$(
    tail -n 1 \
      "${total_file}"
  )"

  over_1s="$(
    awk \
      '$1 >= 1.0 { count++ }
       END { print count + 0 }' \
      "${total_file}"
  )"

  over_5s="$(
    awk \
      '$1 >= 5.0 { count++ }
       END { print count + 0 }' \
      "${total_file}"
  )"

  non_200="$(
    awk \
      -F, \
      -v scale="${scale}" \
      -v case_name="${case_name}" \
      '
        NR > 1 &&
        $1 == scale &&
        $2 == case_name &&
        $9 != 200 {
          count++
        }
        END {
          print count + 0
        }
      ' \
      "${RAW_CSV}"
  )"

  printf \
    "%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n" \
    "${scale}" \
    "${case_name}" \
    "${count}" \
    "${p50_ttfb}" \
    "${p95_ttfb}" \
    "${max_ttfb}" \
    "${p50_total}" \
    "${p95_total}" \
    "${max_total}" \
    "${over_1s}" \
    "${over_5s}" \
    "${non_200}" \
    >> "${SUMMARY_CSV}"

  rm -f \
    "${ttfb_file}" \
    "${total_file}"
}

write_environment

for scale in ${SCALES}; do
  seed_scale "${scale}"

  verify_database_count "${scale}"

  run_explain "${scale}"

  echo
  echo "API benchmark: ${scale} rows"

  benchmark_case \
    "${scale}" \
    "all" \
    "all"

  benchmark_case \
    "${scale}" \
    "category" \
    "matched" \
    --data-urlencode "category=latte_art"

  benchmark_case \
    "${scale}" \
    "title" \
    "matched" \
    --data-urlencode "title=latte-art"

  benchmark_case \
    "${scale}" \
    "title_category" \
    "matched" \
    --data-urlencode "title=latte-art" \
    --data-urlencode "category=latte_art"

  benchmark_case \
    "${scale}" \
    "similar" \
    "similar" \
    --data-urlencode "title=latte-artt"

  summarize_case \
    "${scale}" \
    "all"

  summarize_case \
    "${scale}" \
    "category"

  summarize_case \
    "${scale}" \
    "title"

  summarize_case \
    "${scale}" \
    "title_category"

  summarize_case \
    "${scale}" \
    "similar"
done

echo
echo "========================================"
echo "Search benchmark completed"
echo "========================================"
echo
echo "Results:"
echo "  ${RESULT_DIR}"
echo
echo "Summary:"
echo "  ${SUMMARY_CSV}"
echo
echo "Raw measurements:"
echo "  ${RAW_CSV}"
echo
echo "EXPLAIN ANALYZE:"
echo "  ${RESULT_DIR}/explain_100.txt"
echo "  ${RESULT_DIR}/explain_1000.txt"
echo "  ${RESULT_DIR}/explain_10000.txt"
echo
echo "The final benchmark dataset remains in the database."
echo
echo "Cleanup command:"
echo
echo "docker compose run --rm migrate \\"
echo "  go run ./cmd/seed_search_benchmark \\"
echo "  --confirm-local \\"
echo "  --cleanup"
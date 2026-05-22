#!/usr/bin/env bash
set -euo pipefail

HOST="${SERVERLEDGE_HOST:-localhost}"
PORT="${SERVERLEDGE_PORT:-1323}"
CLI="${SERVERLEDGE_CLI:-bin/serverledge-cli}"
BUILD_IMAGES=false
PUSH_IMAGES=false
UPDATE_FLAG="${SERVERLEDGE_UPDATE_FLAG:---update}"

usage() {
  printf 'Usage: %s [--host HOST] [--port PORT] [--cli PATH] [--build-images] [--push-images] [--no-update]\n' "$0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host)
      HOST="$2"
      shift 2
      ;;
    --port)
      PORT="$2"
      shift 2
      ;;
    --cli)
      CLI="$2"
      shift 2
      ;;
    --build-images)
      BUILD_IMAGES=true
      shift
      ;;
    --push-images)
      BUILD_IMAGES=true
      PUSH_IMAGES=true
      shift
      ;;
    --no-update)
      UPDATE_FLAG=""
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      printf 'Unknown argument: %s\n' "$1" >&2
      exit 2
      ;;
  esac
done

if [[ ! -x "$CLI" ]]; then
  printf 'Serverledge CLI not found or not executable: %s\n' "$CLI" >&2
  exit 1
fi

run() {
  printf '\n+'
  printf ' %q' "$@"
  printf '\n'
  "$@"
}

build_image() {
  local tag="$1"
  local context="$2"

  run docker build -t "$tag" "$context"
  if [[ "$PUSH_IMAGES" == true ]]; then
    run docker push "$tag"
  fi
}

create_function() {
  run "$CLI" --host "$HOST" --port "$PORT" create "$@"
}

if [[ "$BUILD_IMAGES" == true ]]; then
  build_image ceciliacal/my_python310 images/my_python310/
  build_image ceciliacal/ml-py images/my_ml/
  build_image ceciliacal/video-py images/video/
fi

create_function \
  $UPDATE_FLAG \
  -f euler_standard \
  --memory 100 \
  --src examples/function_variant/euler_standard.py \
  --runtime my_python310 \
  --handler "euler_standard.handler"

create_function \
  $UPDATE_FLAG \
  -f euler_standard \
  --variant euler_light \
  --memory 100 \
  --src examples/function_variant/euler_light.py \
  --runtime my_python310 \
  --handler "euler_light.handler" \
  --speedup 2.0 \
  --utility 0.8

create_function \
  $UPDATE_FLAG \
  -f ml_standard \
  --memory 1500 \
  --src examples/function_variant/ml_standard.py \
  --runtime ml-py \
  --handler "ml_standard.handler"

create_function \
  $UPDATE_FLAG \
  -f ml_standard \
  --variant ml_light \
  --memory 1500 \
  --src examples/function_variant/ml_light.py \
  --runtime ml-py \
  --handler "ml_light.handler" \
  --speedup 2.1 \
  --utility 0.5

create_function \
  $UPDATE_FLAG \
  -f video_standard \
  --memory 800 \
  --src examples/function_variant/video_standard.py \
  --runtime video-py \
  --handler "video_standard.handler"

create_function \
  $UPDATE_FLAG \
  -f video_standard \
  --variant video_light \
  --memory 800 \
  --src examples/function_variant/video_light.py \
  --runtime video-py \
  --handler "video_light.handler" \
  --speedup 2.0 \
  --utility 0.7

#!/bin/bash
GPU=""
BASE_IMAGE="rayproject/ray:latest"

while [[ $# -gt 0 ]]; do
  case $1 in
    --gpu)
      GPU="-gpu"
      BASE_IMAGE="rayproject/ray:latest-gpu"
      shift
      ;;
    --base-image)
      BASE_IMAGE="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

export DOCKER_BUILDKIT=1
docker build \
  --build-arg BASE_IMAGE="$BASE_IMAGE" \
  --build-arg GPU="$GPU" \
  -t "subnet-ray-node:latest$GPU" \
  -f Dockerfile .

#!/bin/sh
set -eu

uvicorn recommendation_service.app:app --host 0.0.0.0 --port "${PORT:-7860}"

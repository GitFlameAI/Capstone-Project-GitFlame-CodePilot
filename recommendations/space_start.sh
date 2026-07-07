#!/bin/sh
set -eu

uvicorn recommendation_service.app:app --host 0.0.0.0 --port 7860

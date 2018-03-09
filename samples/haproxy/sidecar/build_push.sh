#!/bin/sh
docker build -t etiennecoutaud/metrics-sidecar:latest .
docker push etiennecoutaud/metrics-sidecar:latest

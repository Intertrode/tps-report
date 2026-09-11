# tps-report

[![CircleCI](https://dl.circleci.com/status-badge/img/gh/Intertrode/tps-report/tree/main.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/Intertrode/tps-report/tree/main)

CircleCI Field Engineer take-home exercise: a reference pipeline that builds a custom Docker image, tests it against a real Postgres sidecar, and publishes it to Amazon ECR via OIDC — with dynamic config that skips build/test/publish entirely on docs-only changes.

See **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** for the full writeup: architecture, component mapping, CircleCI value/optimizations, and future trade-offs.

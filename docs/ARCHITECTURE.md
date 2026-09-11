# TPS Report: A CircleCI Reference Pipeline

**Repository:** https://github.com/Intertrode/tps-report
**Passing build (full pipeline, publish included):** https://app.circleci.com/pipeline/99b67926-bf44-4fda-bd2e-24aaec338822
**Passing build (conditional-skip proof, docs-only change):** https://app.circleci.com/pipeline/72a74f47-1f18-4023-8cc8-6a09b8629a20

---

## 1. Overview

This repository is a reference CircleCI pipeline built around a small Go HTTP service. The point of the exercise isn't the application — it's everything CircleCI does around it: build a custom Docker image in-pipeline, test it against a real database sidecar, publish it to a cloud registry using zero long-lived credentials, and skip all of that work automatically when a change doesn't warrant it.

Every claim in this document is backed by a real, inspectable CircleCI run — not a description of intended behavior. Where useful, this doc cites the actual job ID, commit SHA, or measured output that proves a given piece works, rather than asking you to take it on faith.

## 2. Architecture

The pipeline is split into two CircleCI configs, which is itself part of the design (see §4.1):

```
GitHub push (Intertrode/tps-report, any branch)
        │
        ▼
┌────────────────────────────────────────────────────────────┐
│  .circleci/config.yml  (setup config, setup: true)          │
│  circleci/path-filtering@3.0.0 diffs the commit against     │
│  main and sets pipeline parameter run-build = true only if  │
│  src/, Dockerfile, scripts/, .circleci/, or aws/ changed.    │
└────────────────────────────────────────────────────────────┘
        │
        ├── run-build = false ──────────────────────────────┐
        │                                                     ▼
        │                                    workflow: skipped-docs-only-change
        │                                    job: skip-notice (explains why nothing ran)
        │
        └── run-build = true
                │
                ▼
┌────────────────────────────────────────────────────────────┐
│  .circleci/continue_config.yml   workflow: baseline-verification│
│                                                               │
│   smoke-test ──────────┐                                    │
│                         │                                    │
│   test-integration      │   (Go 1.26 + Postgres 16 sidecar,  │
│   ├─ go test via         │    JUnit results collected)        │
│   │  gotestsum            │                                   │
│   ▼                       │                                   │
│   build-docker-image ◄────┘  (requires test-integration)      │
│   ├─ docker build (setup_remote_docker)                       │
│   ├─ real container smoke test: Postgres + app on a           │
│   │  Docker network, curl /healthz                            │
│   ▼                                                             │
│   publish-to-ecr  (requires build-docker-image)                │
│   ├─ context: aws-ecr-publish, branch filter: main only       │
│   ├─ OIDC: sts:AssumeRoleWithWebIdentity via $CIRCLE_OIDC_TOKEN│
│   └─ docker push → Amazon ECR                                 │
└────────────────────────────────────────────────────────────┘
                │
                ▼
        Amazon ECR: 916028337694.dkr.ecr.us-east-1.amazonaws.com/tps-report
        (tagged with the commit SHA and :latest)
```

### Component mapping

| Component | Role | Talks to |
|---|---|---|
| GitHub App (`circleci-app`) | Delivers push events to CircleCI | GitHub ↔ CircleCI |
| `path-filtering` orb | Decides whether the real pipeline needs to run | Compares `git diff` against `main`, sets a pipeline parameter |
| `continuation` orb (used internally by `path-filtering`) | Hands off from the setup config to the real config | CircleCI-internal |
| `smoke-test` job | Cheap sanity check that checkout + VCS metadata work | Runs standalone, `cimg/base` |
| `test-integration` job | Runs the Go test suite against a live database | Primary container (`cimg/go`) ↔ secondary/sidecar container (`cimg/postgres`) over `127.0.0.1:5432` |
| `build-docker-image` job | Builds the image, then proves it actually works | `setup_remote_docker` (isolated Docker daemon) → a hand-built Docker network with a fresh Postgres container + the just-built app image |
| `publish-to-ecr` job | Authenticates to AWS and ships the image | `circleci/aws-cli` orb → AWS STS (OIDC) → Amazon ECR |
| CircleCI Context `aws-ecr-publish` | Holds the one non-secret credential (an IAM role ARN) the publish job needs, gated two ways | Restricted to this project + to `main`-branch pipelines only |
| AWS IAM OIDC provider + role | Trusts CircleCI's OIDC issuer for this org, scoped to this exact project via the `sub` claim | No static AWS keys exist anywhere in this system |

## 3. What each job does

- **`smoke-test`** — checks out the repo and echoes branch/commit metadata. Exists as a fast, near-zero-cost signal that the VCS connection itself is healthy, independent of anything else.
- **`test-integration`** — the real test suite (`src/main_test.go`): a health-check test and two tests that do genuine GORM `AutoMigrate` + CRUD against the Postgres sidecar over a TCP socket, not mocks. Results go through `gotestsum` to produce JUnit XML, which CircleCI's `store_test_results` step ingests — CircleCI's Tests tab shows all three tests individually with real timings (0.02s–0.07s each), not just a pass/fail job outcome.
- **`build-docker-image`** — builds the multi-stage image (`golang:1.26` builder → `gcr.io/distroless/static-debian12` final) inside an isolated remote Docker environment, then runs a genuine smoke test: a fresh Postgres container and the just-built app image are started on a private Docker network, and `/healthz` is polled with `curl` until the app actually responds `{"status":"ok"}` — proving the built artifact works, not just that it compiled.
- **`publish-to-ecr`** — loads the image built in the previous job (handed off via a CircleCI workspace, since each job gets its own isolated Docker daemon), assumes an AWS IAM role using the pipeline's short-lived `$CIRCLE_OIDC_TOKEN`, and pushes the image to Amazon ECR under both the commit SHA and `:latest`.

## 4. CircleCI value and optimizations demonstrated

**4.1 — Real dynamic configuration, not a shortcut.** Conditional execution is implemented with CircleCI's actual dynamic-config primitives (`setup: true`, the `path-filtering` and `continuation` orbs), not a hand-rolled `git diff` check. This was verified in both directions with live pushes: a commit touching pipeline-relevant files ran the full four-job pipeline; a commit touching only `README.md` ran *just* the path-filtering job and a `skip-notice` job — the `baseline-verification` workflow didn't even appear in that run. A reviewer looking at pipeline history for a docs-only change sees an intentional skip, not something that looks broken.

**4.2 — OIDC with zero static cloud credentials.** The publish job never stores an AWS access key anywhere — not in a CircleCI Context, not in the repo, not in an environment variable. It authenticates by presenting CircleCI's own short-lived OIDC token to AWS STS, which hands back credentials valid for one hour. The IAM role's trust policy is scoped down to this exact CircleCI project via the `sub` claim, so even someone with the role's ARN couldn't assume it from anywhere else.

**4.3 — Defense in depth on credentials, not just one gate.** The publish job is gated three separate ways, not one: (a) `requires: build-docker-image` in the DAG, (b) `filters: branches: only: main`, and (c) the Context itself is independently restricted to both this project and to `main`-branch pipelines. Any one of these alone would satisfy "credentials not accessible outside approved builds" — having all three is deliberate.

**4.4 — Genuine sidecar-based integration testing.** The test job doesn't mock the database. A real `cimg/postgres` container runs alongside the test container for the lifetime of the job, and the test suite performs real migrations and real reads/writes against it — the kind of test that actually catches schema or query bugs.

**4.5 — Least-privilege IAM, not a broad managed policy.** The publish role's permissions are scoped to exactly five ECR actions against exactly one repository ARN — not `AmazonEC2ContainerRegistryPowerUser` or similar broad grants.

**4.6 — CircleCI's own AI remediation tooling caught a real bug during this build.** Early in development, the built-in `checkout` step failed with a zero-length deploy key (a consequence of a GitHub organization default that changed in October 2024). CircleCI's `circleci-app[bot]` diagnosed this and opened a pull request with a working fix, unprompted — a live example of the platform's own tooling value, not a hypothetical.

## 5. Future optimizations and trade-offs

- **Docker layer caching** (`docker_layer_caching: true` on `setup_remote_docker`) would meaningfully speed up repeat image builds; it was left off here since it's a paid-tier feature and this pipeline runs on a free-tier org. Worth enabling on any plan that supports it.
- **Test splitting** (`circleci tests split --split-by=timings`) isn't needed yet — three tests running in under a second doesn't justify it — but is the natural next step if the test suite grows into the hundreds of cases.
- **Container-structure testing** (`dgoss` or `container-structure-test`) would validate the built image's filesystem/config directly, complementing the current network-level smoke test with assertions like "the binary is non-root" or "no unexpected packages are present."
- **Deploying the published image**, not just publishing it, is a natural next step once there's a target to deploy to — e.g., ECS Fargate or a container-based Lambda — the ECR publish job here is already the first half of that pipeline.
- **PR-time validation** — the pipeline currently triggers on pushes to any branch and gates *publishing* to `main`, but doesn't yet have a dedicated required-status-check flow for pull requests. Adding a `pull_request` event trigger alongside the current push trigger would let this same pipeline double as a merge gate.
- **Multi-arch builds** (native Arm64 via Graviton-class resource classes) would matter if this service ever needed to run on Arm-based compute; not implemented here since there's no such target yet.
- **Scoping the IAM role further** as the project grows — today's trust policy and permissions policy are both about as tight as they can be for a single-repo, single-registry setup; a multi-service version of this pattern would want per-service roles rather than widening one role's scope.

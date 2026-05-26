---
type: decision
title: AWS deployment — Elastic Beanstalk Docker (ALB, us-east-1)
status: accepted
owner: conductor
workload: sous-chef-ios
last_updated: 2026-05-25
source_pin: d884efae9cc150df2a58afc255b3e631d31b5d2b
---

# ADR-0013 — AWS deployment via Elastic Beanstalk Docker

This ADR pivots the backend track from **ECS Fargate** (per
`plan/track-backend.md` §3.3 as authored by the backend Builder) to
**Elastic Beanstalk** with the **Docker on Amazon Linux 2023** platform,
**load-balanced** behind an **Application Load Balancer**, in **us-east-1**.

The Fargate choice was a Builder track-plan decision, not an Accepted ADR,
so this ADR is the binding direction going forward; the track plan's §3.3
paragraph is amended via a NOTE block pointing at this ADR.

Depends on the workload spec, `plan/track-backend.md` §3 *Architecture* and
§9 *Open questions*, and `plan/integration.md` §3.2 *Dave-actions*.

---

## Context

The backend Builder's track plan §3.3 picked ECS Fargate over Lambda+API
Gateway and EC2-direct. The reasoning was:

- **Lambda + API Gateway breaks SSE** — idle timeout, response buffering,
  and a 30-second response ceiling are incompatible with the contract §6
  chat stream (which can run several minutes).
- **EC2-direct over-provisions** a single-household workload.
- **Fargate** holds TCP open through the ALB→container path verbatim,
  keeps a warm `*http.Client` to OpenAI between requests, and gives us
  "compute as a unit" without ASG management.

All three points still apply. The pivot to Beanstalk does not re-open the
SSE-friendly compute decision; it changes **how the same container is
operated**:

| Dimension | ECS Fargate | Elastic Beanstalk Docker |
|---|---|---|
| Underlying compute | Fargate task | EC2 instance(s) in an ASG |
| Container runtime | Fargate's | Docker on AL2023 (the platform image) |
| Image source | ECR | ECR (via `Dockerrun.aws.json`) or local push |
| Load balancer | ALB (separate stack) | ALB (provisioned by EB) |
| Deploy CLI | `aws ecs update-service` | `eb deploy` |
| Per-month baseline | ~$8–12 (one Fargate task) | ~$15–20 (one t3.small + ALB) |
| Operator surface to learn | ECS task defs + IAM | `eb` CLI + EB console |
| Native SSE compatibility | Same (both behind ALB) | Same (both behind ALB) |
| Distroless runtime | ✓ | ✓ (ALB health-check via /healthz, not SSH exec) |

Dave's reasons for the pivot (captured at the gate):

- Familiarity / fit for a personal-app operator surface: `eb deploy` is
  one verb; ECS service updates are multi-step.
- Smaller blast radius when something goes wrong — Beanstalk's "swap
  environment URLs" rollback is more intuitive than ECS's
  `--force-new-deployment` mental model.
- Easier to operate without infrastructure-as-code in the early phase
  (no Terraform / CDK / CloudFormation overhead — EB's environment
  config is plain YAML the CLI manages).

## Decision

**The backend deploys to AWS Elastic Beanstalk, on the Docker on Amazon
Linux 2023 platform, behind an Application Load Balancer, in us-east-1.**

Concretely:

1. **Platform**: `Docker running on 64bit Amazon Linux 2023` (latest).
   The container runs from the same multi-stage Dockerfile already in
   `backend/Dockerfile` — multi-stage `golang:1.26.3-alpine` →
   `gcr.io/distroless/static-debian12:nonroot`. No changes to the
   image.
2. **Image source**: pushed to ECR (`sous-chef-api` repository in
   us-east-1). Beanstalk receives a `Dockerrun.aws.json` v3 in the
   deploy bundle pointing at the ECR image; the EB-managed Docker
   daemon pulls and runs it. This separation (ECR holds the artifact,
   EB holds the environment) matches AWS's documented pattern and lets
   the same image run locally for smoke tests.
3. **Environment tier**: **load-balanced**. ALB in front, one
   EC2 instance behind (auto-scaling group min=1, max=1 for v1; max
   bumps later if traffic warrants). ALB is the SSL terminator.
4. **ALB idle timeout**: **600 seconds** (10 minutes), set via an
   `.ebextensions/` config file so the value is version-controlled.
   The backend's SSE writer ships a 20-second heartbeat per the track
   plan, well inside this window.
5. **Health checks**: ALB target group hits `GET /healthz` every 30s,
   2 unhealthy → instance replaced. EB's own platform-level
   enhanced-health uses the same endpoint.
6. **Region**: **us-east-1** (N. Virginia). Lowest cost, widest service
   availability, closest to OpenAI's primary endpoints.
7. **Three environments**: `sous-chef-api-dev`,
   `sous-chef-api-staging`, `sous-chef-api-prod`, each its own EB
   environment under one EB application (`sous-chef-api`). Promotion
   is `eb deploy <env-name>` with the same image tag.

The configuration above lives in two places in the backend track's
output repo (added in Phase L per `plan/track-backend.md` §5):

- `backend/.elasticbeanstalk/config.yml` — EB CLI config, region, app
  name, default environment.
- `backend/.ebextensions/01-alb-idle-timeout.config` — the 600-second
  idle-timeout setting, as YAML the EB platform applies.

Neither file lands in this ADR — they ship with the first real deploy
(Phase L task).

## Status

**Accepted.** Locked by Dave at the AWS-deployment decision point,
2026-05-25.

## Consequences

### Positive

- **One CLI to learn** (`eb`) for the entire deploy lifecycle:
  `eb init`, `eb create`, `eb deploy`, `eb logs`, `eb terminate`. No
  separate ECS / ECR / IAM / CloudWatch knobs to learn for the
  happy path.
- **Rollback is intuitive**: `eb deploy --version <older>` re-deploys
  a known-good image tag. The EB console also shows version history.
- **Infrastructure-as-code in the same repo** via `.ebextensions/`
  (YAML files committed alongside the source). No separate Terraform
  module to maintain in Phase 4.
- **Distroless image still works** — EB's ALB health checks the
  application over HTTP (`/healthz`), not via SSH-exec, so the lack
  of a shell in the runtime image is not a constraint.
- **The Dockerfile we already have is the deploy artifact.** No
  Phase L scaffolding rewrite.

### Negative

- **~$5–8/month more than Fargate** for the same traffic shape (the
  ALB is included in EB's bill structure rather than provisioned
  separately, but the always-on EC2 instance has a higher floor than
  Fargate's pay-per-second). For a personal-app workload this is
  invisible.
- **Less granular control** over the underlying EC2 instance (the EB
  platform image manages OS patching, Docker daemon, etc.). When
  something breaks at that layer, the Beanstalk-specific failure modes
  are different from running on raw ECS. Mitigation: stick to the
  default platform image and the documented `.ebextensions/`
  patterns; do not customise the AMI.
- **`eb` CLI is older than the modern AWS-CLI v2 / CDK ecosystem.**
  Some patterns (immutable env updates, blue/green via env swap) are
  EB-specific idioms a new developer must learn. Documented in the
  Phase-L deploy guide.

### Neutral / preserved

- **SSE works identically** to the Fargate plan. ALB idle-timeout
  knob is the only setting that matters; we pin 600s.
- **The Go binary, Dockerfile, Makefile, and ECR-as-image-source story
  carry over unchanged.** Nothing in `backend/` needs to change today.
- **us-east-1 was the implicit default** in the track-backend plan;
  this ADR makes it explicit.

### What this implies for the backend track plan

`plan/track-backend.md` §3.3 *AWS target* is amended via a NOTE block at
the top of the plan pointing at this ADR. The plan's task list (§5)
gains two Phase-L tasks (already implied; now explicit):

- L1: `eb init` against `sous-chef-api` application in us-east-1; commit
  `.elasticbeanstalk/config.yml`.
- L2: write `.ebextensions/01-alb-idle-timeout.config` setting ALB idle
  timeout to 600s; commit it.

The other Phase-L tasks (CI builds image, pushes to ECR, `eb deploy`)
stay as written.

### What this implies for Dave-actions

`plan/integration.md` §3.2 *Dave-actions* was tracking
"confirm AWS staging ALB idle-timeout settable to 600s" as a blocker
on the Fargate path. That action is **superseded** by this ADR — the
600s value is now version-controlled in `.ebextensions/` and lands when
EB creates the environment.

The new equivalent Dave-action is **create the EB application and the
three environments in us-east-1**, which lands in Phase L (when the
backend is ready to deploy). Documented in the build log when Phase L
opens.

### Reversibility

Pivoting back to ECS Fargate (or forward to a different compute, e.g.
Lightsail Containers, App Runner) is a Phase-L-internal change: drop
`.elasticbeanstalk/` and `.ebextensions/`, write the ECS service
definition (or App Runner config), keep the same Dockerfile. **The
backend Go code does not change.** The reversibility is meaningful —
this is an ops choice, not a contract choice.

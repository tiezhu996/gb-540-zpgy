# ParcelGraph: Cadastral Boundary Topology Resolution

ParcelGraph is an offline decision-support tool for surveying firms and natural-resource data-governance teams. It retains parcel boundaries, observations, proposals, and topology conflicts as reviewable evidence. It does not write to statutory cadastral-registration systems, determine ownership, or publish legal conclusions.

## Docker Quick Start

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps
```

After all services are healthy, visit [http://localhost:18540](http://localhost:18540). Backend liveness is `http://localhost:19540/healthz`; the frontend proxy exposes `http://localhost:18540/api/healthz`.

All demo accounts use `DemoPass123!`.

| Account | Role | Primary scope |
| --- | --- | --- |
| `surveyor` | `surveyor` | Create parcels and import observations |
| `gis_analyst` | `gis_analyst` | Create proposals and run detection |
| `reviewer` | `reviewer` | Review proposals and conflicts |
| `auditor` | `auditor` | Read audit evidence |
| `admin` | `admin` | Full access |

Stop and remove local data:

```bash
docker compose down -v --remove-orphans
```

## Workflow

1. **LandParcel** stores a code, projected coordinate system, GeoJSON boundary, computed area, version, owner organization, and state.
2. **SurveyObservation** imports a point with capture time, method, horizontal accuracy, checksum, quality note, and actor.
3. **BoundaryProposal** references a parcel version and observations, preserving proposed geometry, snap tolerance, rationale, and review state.
4. **TopologyConflict** records geometry, severity, suggested resolution, algorithm version, input hash, and an immutable resolution trail.

Live pages are `/parcels`, `/observations`, `/proposals`, `/conflicts`, and `/audit`; all consume `/api/v1` rather than static mocks.

## Architecture

| Layer | Implementation |
| --- | --- |
| Frontend | Vue 3, TypeScript, Vite, Element Plus, Pinia |
| Backend | Go 1.22, Gin, GORM, validator/v10, JWT, slog |
| Production database | PostgreSQL 16 + PostGIS 3.4 with spatial columns and GiST indexes |
| Smoke database | SQLite using the same Go GeoJSON validation and topology service |
| Deployment | Docker Compose and Nginx |

```text
backend/cmd/server                 composition and graceful shutdown
backend/internal/{model,dto}       persistence entities and HTTP contracts
backend/internal/repository        entity-specific GORM access and transactions
backend/internal/service           validation, state machines, audit writes
backend/internal/geometry          structural GeoJSON and planar calculations
backend/internal/{handler,router}  HTTP envelopes, JWT, RBAC routing
backend/internal/middleware        request ID, recovery, auth, RBAC, audit context, JSON errors
frontend/src/{types,api,stores}    entity-specific client contracts and state
frontend/src/components/common     legend, proposal badge, evidence drawer
frontend/src/pages                 five operational pages and login
```

In PostgreSQL deployments, application migration materializes the validated GeoJSON fields into PostGIS geometry columns, maintains them with triggers, and adds GiST indexes. The `coordinate_system` field remains the authoritative CRS metadata; PostGIS storage does not transform coordinates or turn a suggestion into a legal boundary.

## API

All endpoints below except login and health checks require `Authorization: Bearer <JWT>` and start at `/api/v1`.

| Method | Path | Use |
| --- | --- | --- |
| `POST` | `/auth/login` | Authenticate and obtain role-bound JWT |
| `GET`, `POST` | `/parcels` | List or create parcels |
| `GET`, `PATCH` | `/parcels/:id` | Read or version-update a parcel |
| `GET` | `/observations` | List observations |
| `POST` | `/observations/import` | Import an observation |
| `POST` | `/observations/:id/transition` | Accept, reject, or supersede an observation |
| `GET`, `POST` | `/proposals` | List or create proposals |
| `POST` | `/proposals/:id/transition` | Move proposal through allowed states |
| `GET` | `/conflicts` | List detected topology conflicts |
| `POST` | `/conflicts/detect` | Run detection; requires an `Idempotency-Key` header |
| `POST` | `/conflicts/:id/transition` | Confirm, mark false positive, propose resolution, or close a conflict |
| `POST` | `/conflicts/:id/apply-suggestion` | Create a new draft proposal from the reviewed suggestion and resolve the source conflict |
| `GET` | `/audit` | Read immutable audit events |

`/healthz` is liveness; `/readyz` verifies database readiness.

The frontend sends every request through `/api/v1`. `parcel_ids` is persisted by the backend as a JSON array encoded in text and is normalized to a numeric array in the conflict API client before pages or Pinia consume it, so every participating parcel can be labeled consistently.

## Shared Enums And State

`ConflictType = overlap | gap | self_intersection | dangling_edge`

- Backend: `backend/internal/constants/conflict_type.go`, conflict DTO input, `model/topology_conflict.go`, `geometry/geometry.go`, `service/topology_conflict_service.go`, `repository/topology_conflict_repository.go`, `handler/topology_conflict_handler.go`, `router/topology_conflict_router.go`, and `geometry/topology_test.go`.
- Frontend: `frontend/src/types/enums/conflict-type.ts`, entity type, API, Pinia store, `TopologyLegend.vue`, and `ConflictsPage.vue`.

`ProposalState = draft | validated | submitted | reviewed | revision | accepted | rejected`

- Backend: `backend/internal/constants/proposal_state.go`, proposal DTO input, model, `service/boundary_proposal_service.go`, repository, handler, router, and `service/cadastral_service_test.go`.
- Frontend: `frontend/src/types/enums/proposal-state.ts`, entity type, API, Pinia store, `ProposalStateBadge.vue`, and `ProposalsPage.vue`.

The independent Gin middleware files are `request_id.go`, `recovery.go`, `auth.go`, `rbac.go`, `audit.go`, and `error_handler.go`. They establish request correlation and audit context before authentication, enforce authorization and rate limits, recover panics, and retain a uniform JSON fallback for recorded Gin errors.

Allowed proposal flow is `draft -> validated -> submitted -> reviewed -> accepted/rejected`, with `reviewed -> revision -> draft`. Illegal transitions return `409`; an author cannot review their own proposal. Conflict flow is `detected -> confirmed -> resolution_proposed -> resolved -> closed`, with the alternate `detected -> false_positive -> closed` path. Applying a reviewed suggestion creates a new draft proposal version and resolves the source conflict; it does not rewrite the original proposal or parcel boundary.

## Coordinates And Legal Boundary

- GeoJSON is parsed structurally, never assembled with string operations.
- Parcel boundaries must be closed, finite, non-self-intersecting `Polygon`s; observations must be finite `Point`s.
- Only projected EPSG systems are accepted for meter calculations. `EPSG:4326`, `EPSG:4490`, and `EPSG:4269` are rejected so degrees are never used as meters.
- Original geometry is kept. The deterministic suggestion prefers a matching reference vertex, then the nearest projection on a reference edge, and records every moved coordinate with its distance and target kind.
- Detection stores tolerance, algorithm version, input hash, and immutable result IDs. `Idempotency-Key` is required (1-128 characters): replaying the same actor/key/request returns the stored result set, while reusing that key with a different detection request returns `409`.
- No result is a legal boundary establishment, title decision, or registration action.

## Environment And Ports

| Variable | Default | Meaning |
| --- | --- | --- |
| `COMPOSE_PROJECT_NAME` | `cadastral-boundary-topology-resolution` | Compose namespace |
| `FRONTEND_PORT` | `18540` | Nginx host port |
| `BACKEND_PORT` | `19540` | Gin host port |
| `DB_PORT` | `57540` | PostgreSQL host port |
| `DB_DRIVER` | `postgres` | Production database driver |
| `JWT_SECRET` | `.env` | Production HS256 secret, at least 32 chars |
| `CORS_ORIGINS` | local frontend URLs | Comma-separated allowed origins |

`.env` is untracked; `.env.example` lists Compose values.

For direct cross-origin API use, CORS permits `Authorization`, `Content-Type`, `X-Request-ID`, and `Idempotency-Key` request headers. The Compose frontend normally uses the same-origin Nginx `/api` proxy.

## Local Development And Validation

```bash
go work sync
go build ./backend/...
go vet ./backend/...
go test ./backend/...
go test -race ./backend/...
npm --prefix frontend ci
npm --prefix frontend run build


docker compose config --quiet
```

`runtime_smoke.json` starts the Go service on port `20540` using in-memory SQLite and a prescribed throwaway secret. It makes an actual HTTP health request rather than accepting a mocked result.

## Troubleshooting

- For unhealthy Compose services, use `docker compose logs db backend frontend` and check ports `18540`, `19540`, and `57540`.
- `422` means invalid GeoJSON or coordinate reference system.
- `409` means an illegal or stale state/version transition.
- `403` means the authenticated role is not allowed to perform that action.

## License

MIT. See [LICENSE](./LICENSE).

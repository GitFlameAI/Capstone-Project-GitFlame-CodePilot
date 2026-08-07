# Open decisions

These items need an explicit product or deployment decision. Keep this file
until each item is resolved so the questions do not get lost between handoffs.

## Product and deployment

1. **Root README.** There is no root `README.md`. Decide whether to create one
   from `docs/README-SECTION-TO-ADD.md` or merge that section into a README from
   another source; delete the staging file afterwards.
2. **Alert delivery.** Alertmanager currently uses `null-receiver`. Choose the
   operational channel (Telegram or Slack) and configure the corresponding
   receiver in `infra/observability/alertmanager.yml`.
3. **Operations authorization.** `/ops` currently accepts any valid application
   session. Decide whether that is sufficient or whether production requires an
   operator-only role or a separate `OPS_TOKEN`.
4. **Grafana exposure.** Local development currently falls back to
   `GRAFANA_PASSWORD=admin`. Before publishing port 3000, set a non-default
   secret and decide whether the port should be externally reachable at all.

## Acceptance prerequisite

5. **GitFlame end-to-end test.** Provide a valid local `GITFLAME_API_KEY` and a
   disposable repository/issue with `.ai.yml`. Then verify real background
   indexing (`running` to `completed`), plan generation waiting for it, worker
   shutdown during generation, and one `request_id` across backend, worker and
   agent-engine.

## Already resolved locally

- Migration 006 has been applied to the existing local PostgreSQL database.

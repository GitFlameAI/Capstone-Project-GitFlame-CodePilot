-- Keep the persisted repository lifecycle aligned with the RAG indexer and
-- make one recommendation finding unique inside a single analysis run.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE repository_snapshots
    DROP CONSTRAINT IF EXISTS repository_snapshots_status_check;

ALTER TABLE repository_snapshots
    ADD CONSTRAINT repository_snapshots_status_check
    CHECK (status IN ('fetched', 'indexed', 'failed'));

ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS finding_fingerprint TEXT;

-- Older rows predate analyzer fingerprints. Give them a deterministic identity
-- so the column can become mandatory without discarding historical reports.
UPDATE recommendations
SET finding_fingerprint = encode(
    digest(
        concat_ws(
            E'\x1f',
            lower(trim(category)),
            replace(trim(file_path), E'\\', '/'),
            COALESCE(line_number::text, ''),
            lower(regexp_replace(trim(problem), E'\\s+', ' ', 'g')),
            lower(regexp_replace(trim(suggestion), E'\\s+', ' ', 'g'))
        ),
        'sha256'
    ),
    'hex'
)
WHERE finding_fingerprint IS NULL
   OR finding_fingerprint !~ '^[0-9a-f]{64}$';

-- Prefer a visible card when an existing run already contains exact duplicates,
-- then preserve the oldest remaining row and its status history.
WITH ranked_recommendations AS (
    SELECT
        id,
        row_number() OVER (
            PARTITION BY recommendation_run_id, finding_fingerprint
            ORDER BY
                CASE current_status
                    WHEN 'open' THEN 0
                    WHEN 'closed' THEN 1
                    ELSE 2
                END,
                created_at,
                id
        ) AS duplicate_rank
    FROM recommendations
)
DELETE FROM recommendations recommendation
USING ranked_recommendations ranked
WHERE recommendation.id = ranked.id
  AND ranked.duplicate_rank > 1;

ALTER TABLE recommendations
    ALTER COLUMN finding_fingerprint SET NOT NULL;

ALTER TABLE recommendations
    DROP CONSTRAINT IF EXISTS recommendations_finding_fingerprint_check;

ALTER TABLE recommendations
    ADD CONSTRAINT recommendations_finding_fingerprint_check
    CHECK (finding_fingerprint ~ '^[0-9a-f]{64}$');

CREATE UNIQUE INDEX IF NOT EXISTS uq_recommendations_run_finding
    ON recommendations(recommendation_run_id, finding_fingerprint);

COMMIT;

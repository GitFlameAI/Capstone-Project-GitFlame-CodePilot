INSERT INTO repositories (
    id,
    external_id,
    name,
    owner,
    default_branch,
    web_url
) VALUES (
    '11111111-1111-1111-1111-111111111111',
    'demo-repo',
    'codepilot-demo',
    'gitflame',
    'main',
    'https://gitflame.local/gitflame/codepilot-demo'
) ON CONFLICT (external_id) DO UPDATE SET
    name = EXCLUDED.name,
    owner = EXCLUDED.owner,
    default_branch = EXCLUDED.default_branch,
    web_url = EXCLUDED.web_url,
    updated_at = now();

INSERT INTO ai_configs (
    id,
    repository_id,
    raw_yml,
    parsed_config_json,
    is_valid,
    retention_days
) VALUES (
    '22222222-2222-2222-2222-222222222222',
    '11111111-1111-1111-1111-111111111111',
    'version: 1
recommendations:
  retention_days: 30',
    '{"version":"1","recommendations":{"retention_days":30}}'::jsonb,
    true,
    30
) ON CONFLICT (id) DO UPDATE SET
    raw_yml = EXCLUDED.raw_yml,
    parsed_config_json = EXCLUDED.parsed_config_json,
    is_valid = EXCLUDED.is_valid,
    retention_days = EXCLUDED.retention_days;

INSERT INTO issue_sessions (
    id,
    repository_id,
    ai_config_id,
    external_issue_id,
    issue_title,
    issue_body,
    issue_author,
    status,
    current_revision
) VALUES (
    '33333333-3333-3333-3333-333333333333',
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222222',
    'ISSUE-101',
    'Replace MemoryStore with PostgreSQL storage',
    'Persist issue workflow state after backend restart.',
    'amir',
    'plan_generated',
    2
) ON CONFLICT (repository_id, external_issue_id) DO UPDATE SET
    issue_title = EXCLUDED.issue_title,
    issue_body = EXCLUDED.issue_body,
    issue_author = EXCLUDED.issue_author,
    status = EXCLUDED.status,
    current_revision = EXCLUDED.current_revision,
    updated_at = now();

INSERT INTO generated_plans (
    id,
    issue_session_id,
    plan_markdown,
    current_revision
) VALUES (
    '44444444-4444-4444-4444-444444444444',
    '33333333-3333-3333-3333-333333333333',
    '# Implementation Plan

1. Add PostgreSQL storage.
2. Save issue sessions and plan revisions.
3. Verify persistence after backend restart.',
    2
) ON CONFLICT (issue_session_id) DO UPDATE SET
    plan_markdown = EXCLUDED.plan_markdown,
    current_revision = EXCLUDED.current_revision,
    updated_at = now();

INSERT INTO agent_tasks (
    id,
    issue_session_id,
    generated_plan_id,
    task_type,
    status,
    tool_execution_summary,
    started_at,
    completed_at
) VALUES (
    '55555555-5555-5555-5555-555555555555',
    '33333333-3333-3333-3333-333333333333',
    '44444444-4444-4444-4444-444444444444',
    'plan_revision',
    'completed',
    'read_file: 2 calls; grep: 1 call; reasoning was not stored.',
    now(),
    now()
) ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    tool_execution_summary = EXCLUDED.tool_execution_summary,
    updated_at = now();

INSERT INTO agent_task_statuses (
    agent_task_id,
    status,
    message
) VALUES
    (
        '55555555-5555-5555-5555-555555555555',
        'queued',
        'Agent task queued.'
    ),
    (
        '55555555-5555-5555-5555-555555555555',
        'processing',
        'Plan revision generation started.'
    ),
    (
        '55555555-5555-5555-5555-555555555555',
        'completed',
        'Plan revision generated after user correction feedback.'
    );

INSERT INTO plan_revisions (
    id,
    generated_plan_id,
    issue_session_id,
    agent_task_id,
    revision_number,
    plan_markdown,
    correction_feedback,
    source
) VALUES (
    '66666666-6666-6666-6666-666666666666',
    '44444444-4444-4444-4444-444444444444',
    '33333333-3333-3333-3333-333333333333',
    '55555555-5555-5555-5555-555555555555',
    2,
    '# Implementation Plan

Updated plan with persistence verification.',
    'Add a restart verification step.',
    'correction'
) ON CONFLICT (generated_plan_id, revision_number) DO UPDATE SET
    plan_markdown = EXCLUDED.plan_markdown,
    correction_feedback = EXCLUDED.correction_feedback,
    source = EXCLUDED.source;

INSERT INTO recommendation_runs (
    id,
    repository_id,
    ai_config_id,
    summary,
    status,
    retention_days,
    expires_at
) VALUES (
    '77777777-7777-7777-7777-777777777777',
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222222',
    'Recommendation report is stored with an explicit retention period.',
    'completed',
    30,
    now() + interval '30 days'
) ON CONFLICT (id) DO UPDATE SET
    summary = EXCLUDED.summary,
    status = EXCLUDED.status,
    retention_days = EXCLUDED.retention_days,
    expires_at = EXCLUDED.expires_at,
    updated_at = now();

SELECT
    'issue workflow persisted' AS verification_case,
    r.external_id AS repository,
    s.external_issue_id AS issue,
    s.status AS session_status,
    s.current_revision,
    gp.current_revision AS plan_revision,
    at.status AS task_status,
    at.started_at IS NOT NULL AS task_started,
    at.completed_at IS NOT NULL AS task_completed,
    pr.correction_feedback
FROM issue_sessions s
JOIN repositories r ON r.id = s.repository_id
JOIN generated_plans gp ON gp.issue_session_id = s.id
JOIN plan_revisions pr ON pr.issue_session_id = s.id
JOIN agent_tasks at ON at.id = pr.agent_task_id
WHERE s.external_issue_id = 'ISSUE-101';

SELECT
    'agent task lifecycle persisted' AS verification_case,
    at.task_type,
    ats.status,
    ats.message,
    ats.created_at
FROM agent_tasks at
JOIN agent_task_statuses ats ON ats.agent_task_id = at.id
WHERE at.issue_session_id = '33333333-3333-3333-3333-333333333333'
ORDER BY ats.created_at;

SELECT
    'recommendation retention persisted' AS verification_case,
    r.external_id AS repository,
    rr.status,
    rr.retention_days,
    rr.expires_at > now() AS is_not_expired
FROM recommendation_runs rr
JOIN repositories r ON r.id = rr.repository_id
WHERE rr.id = '77777777-7777-7777-7777-777777777777';

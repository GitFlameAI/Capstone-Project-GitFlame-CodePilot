INSERT INTO repositories (
    id,
    external_id,
    name,
    owner,
    default_branch,
    web_url
) VALUES (
    '11111111-1111-1111-1111-111111111111',
    'verification-demo-repo',
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

INSERT INTO repository_files (
    id,
    repository_id,
    file_path,
    file_name,
    commit_sha
) VALUES (
    '88888888-8888-8888-8888-888888888888',
    '11111111-1111-1111-1111-111111111111',
    'backend/internal/service/workflow.go',
    'workflow.go',
    'demo-sha'
) ON CONFLICT (repository_id, file_path) DO UPDATE SET
    file_name = EXCLUDED.file_name,
    commit_sha = EXCLUDED.commit_sha,
    updated_at = now();

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
    '99999999-9999-9999-9999-999999999999',
    '44444444-4444-4444-4444-444444444444',
    '33333333-3333-3333-3333-333333333333',
    NULL,
    3,
    '# Implementation Plan

User edited plan before approval.',
    '',
    'user_edit'
) ON CONFLICT (generated_plan_id, revision_number) DO UPDATE SET
    plan_markdown = EXCLUDED.plan_markdown,
    correction_feedback = EXCLUDED.correction_feedback,
    source = EXCLUDED.source;

INSERT INTO git_workflow_payloads (
    id,
    issue_session_id,
    agent_task_id,
    branch_name,
    base_branch,
    commit_message,
    pr_title,
    reviewer,
    status
) VALUES (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    '33333333-3333-3333-3333-333333333333',
    '55555555-5555-5555-5555-555555555555',
    'ai/ISSUE-101-replace-memorystore',
    'main',
    'Implement Replace MemoryStore with PostgreSQL storage',
    'Replace MemoryStore with PostgreSQL storage',
    'amir',
    'generated'
) ON CONFLICT (issue_session_id) DO UPDATE SET
    agent_task_id = EXCLUDED.agent_task_id,
    branch_name = EXCLUDED.branch_name,
    base_branch = EXCLUDED.base_branch,
    commit_message = EXCLUDED.commit_message,
    pr_title = EXCLUDED.pr_title,
    reviewer = EXCLUDED.reviewer,
    status = EXCLUDED.status,
    updated_at = now();

DELETE FROM generated_files
WHERE issue_session_id = '33333333-3333-3333-3333-333333333333';

INSERT INTO generated_files (
    issue_session_id,
    agent_task_id,
    file_path,
    action,
    content,
    explanation,
    status,
    validation_error
) VALUES (
    '33333333-3333-3333-3333-333333333333',
    '55555555-5555-5555-5555-555555555555',
    'backend/internal/service/workflow.go',
    'modify',
    'package service',
    'Updates the workflow implementation.',
    'valid',
    ''
);

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
    'repository file paths persisted' AS verification_case,
    r.external_id AS repository,
    rf.file_path,
    rf.commit_sha
FROM repository_files rf
JOIN repositories r ON r.id = rf.repository_id
WHERE r.external_id = 'verification-demo-repo';

SELECT
    'user edited plan persisted' AS verification_case,
    pr.revision_number,
    pr.source,
    pr.plan_markdown LIKE '%User edited%' AS has_user_edit
FROM plan_revisions pr
WHERE pr.generated_plan_id = '44444444-4444-4444-4444-444444444444'
  AND pr.source = 'user_edit';

SELECT
    'code generation payload persisted' AS verification_case,
    gwp.branch_name,
    gwp.base_branch,
    gwp.commit_message,
    gwp.pr_title,
    gwp.reviewer,
    gf.file_path,
    gf.action,
    gf.status,
    gf.validation_error
FROM git_workflow_payloads gwp
JOIN generated_files gf ON gf.issue_session_id = gwp.issue_session_id
WHERE gwp.issue_session_id = '33333333-3333-3333-3333-333333333333';

SELECT
    'recommendation retention persisted' AS verification_case,
    r.external_id AS repository,
    rr.status,
    rr.retention_days,
    rr.expires_at > now() AS is_not_expired
FROM recommendation_runs rr
JOIN repositories r ON r.id = rr.repository_id
WHERE rr.id = '77777777-7777-7777-7777-777777777777';

INSERT INTO repositories (
    id,
    name,
    owner,
    default_branch
) VALUES (
    '11111111-1111-1111-1111-111111111111',
    'codepilot-demo',
    'gitflame',
    'main'
) ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    owner = EXCLUDED.owner,
    default_branch = EXCLUDED.default_branch;

INSERT INTO ai_configs (
    id,
    repository_id,
    raw_yml,
    parsed_config_json,
    is_valid
) VALUES (
    '22222222-2222-2222-2222-222222222222',
    '11111111-1111-1111-1111-111111111111',
    'version: 1
analysis:
  enabled: true
recommendations:
  enabled: true',
    '{"version":1,"analysis":{"enabled":true},"recommendations":{"enabled":true}}'::jsonb,
    true
) ON CONFLICT (id) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    raw_yml = EXCLUDED.raw_yml,
    parsed_config_json = EXCLUDED.parsed_config_json,
    is_valid = EXCLUDED.is_valid;

INSERT INTO issue_sessions (
    id,
    repository_id,
    ai_config_id,
    issue_title,
    issue_body,
    issue_author,
    status
) VALUES (
    '33333333-3333-3333-3333-333333333333',
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222222',
    'Add recommendation widget',
    'Show repository recommendation summary on the Code page.',
    'demo-user',
    'plan_generated'
) ON CONFLICT (id) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    ai_config_id = EXCLUDED.ai_config_id,
    issue_title = EXCLUDED.issue_title,
    issue_body = EXCLUDED.issue_body,
    issue_author = EXCLUDED.issue_author,
    status = EXCLUDED.status;

INSERT INTO generated_plans (
    id,
    issue_session_id,
    plan_markdown
) VALUES (
    '44444444-4444-4444-4444-444444444444',
    '33333333-3333-3333-3333-333333333333',
    '# Implementation Plan

1. Add recommendation summary endpoint.
2. Store recommendation cards.
3. Render the widget in the demo UI.'
) ON CONFLICT (id) DO UPDATE SET
    issue_session_id = EXCLUDED.issue_session_id,
    plan_markdown = EXCLUDED.plan_markdown;

INSERT INTO user_responses (
    id,
    issue_session_id,
    response_type,
    message,
    author
) VALUES (
    '55555555-5555-5555-5555-555555555555',
    '33333333-3333-3333-3333-333333333333',
    'approve',
    'Plan looks good for the Sprint 1 demo.',
    'demo-user'
) ON CONFLICT (id) DO UPDATE SET
    issue_session_id = EXCLUDED.issue_session_id,
    response_type = EXCLUDED.response_type,
    message = EXCLUDED.message,
    author = EXCLUDED.author;

INSERT INTO recommendation_runs (
    id,
    repository_id,
    ai_config_id,
    summary,
    status
) VALUES (
    '66666666-6666-6666-6666-666666666666',
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222222',
    'The repository is ready for the MVP demo, but the recommendation flow needs persistent storage.',
    'completed'
) ON CONFLICT (id) DO UPDATE SET
    repository_id = EXCLUDED.repository_id,
    ai_config_id = EXCLUDED.ai_config_id,
    summary = EXCLUDED.summary,
    status = EXCLUDED.status;

INSERT INTO recommendations (
    id,
    recommendation_run_id,
    file_path,
    line_number,
    category,
    severity,
    problem,
    suggestion,
    current_status
) VALUES (
    '77777777-7777-7777-7777-777777777777',
    '66666666-6666-6666-6666-666666666666',
    'backend/app/api/routes/recommendations.go',
    12,
    'maintainability',
    'medium',
    'Recommendation responses should be stored instead of returned only as mock data.',
    'Save recommendation runs and cards in PostgreSQL before returning them to the UI.',
    'open'
) ON CONFLICT (id) DO UPDATE SET
    recommendation_run_id = EXCLUDED.recommendation_run_id,
    file_path = EXCLUDED.file_path,
    line_number = EXCLUDED.line_number,
    category = EXCLUDED.category,
    severity = EXCLUDED.severity,
    problem = EXCLUDED.problem,
    suggestion = EXCLUDED.suggestion,
    current_status = EXCLUDED.current_status;

INSERT INTO recommendation_statuses (
    id,
    recommendation_id,
    status,
    changed_by
) VALUES (
    '88888888-8888-8888-8888-888888888888',
    '77777777-7777-7777-7777-777777777777',
    'open',
    'system'
) ON CONFLICT (id) DO UPDATE SET
    recommendation_id = EXCLUDED.recommendation_id,
    status = EXCLUDED.status,
    changed_by = EXCLUDED.changed_by;

SELECT
    'issue workflow saved' AS verification_case,
    repositories.owner || '/' || repositories.name AS repository,
    issue_sessions.issue_title,
    issue_sessions.status AS session_status,
    generated_plans.plan_markdown,
    user_responses.response_type
FROM issue_sessions
JOIN repositories ON repositories.id = issue_sessions.repository_id
JOIN generated_plans ON generated_plans.issue_session_id = issue_sessions.id
JOIN user_responses ON user_responses.issue_session_id = issue_sessions.id
WHERE issue_sessions.id = '33333333-3333-3333-3333-333333333333';

SELECT
    'recommendation workflow saved' AS verification_case,
    repositories.owner || '/' || repositories.name AS repository,
    recommendation_runs.status AS run_status,
    recommendation_runs.summary,
    recommendations.file_path,
    recommendations.severity,
    recommendations.current_status
FROM recommendation_runs
JOIN repositories ON repositories.id = recommendation_runs.repository_id
JOIN recommendations ON recommendations.recommendation_run_id = recommendation_runs.id
WHERE recommendation_runs.id = '66666666-6666-6666-6666-666666666666';

ALTER TABLE issue_sessions DROP CONSTRAINT IF EXISTS issue_sessions_status_check;
ALTER TABLE issue_sessions
    ADD CONSTRAINT issue_sessions_status_check CHECK (
        status IN (
            'created',
            'queued',
            'processing',
            'plan_generated',
            'approved',
            'code_generation_queued',
            'code_generation_processing',
            'code_generated',
            'correction_requested',
            'rejected',
            'failed'
        )
    );

ALTER TABLE agent_tasks DROP CONSTRAINT IF EXISTS agent_tasks_type_check;
ALTER TABLE agent_tasks
    ADD CONSTRAINT agent_tasks_type_check CHECK (
        task_type IN (
            'initial_plan',
            'plan_revision',
            'code_generation',
            'recommendation_analysis'
        )
    );

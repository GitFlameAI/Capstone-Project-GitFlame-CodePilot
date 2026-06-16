# User flow: Issue → Implementation Plan → Pull Request

This is the autogeneration scenario. The user creates an issue; the system checks
the `.ai.yml`, asks the ML service for an implementation plan, and only creates a
branch/PR after the user approves. Branch/PR creation is a **mock/contract-level**
service in Sprint 1 because the real GitFlame API may be unavailable.

```mermaid
sequenceDiagram
    actor User
    participant GF as GitFlame UI
    participant BE as Backend (Go)
    participant ML as ML Service (autogen)
    participant Git as Git Workflow (mock)

    User->>GF: Create issue
    GF->>BE: POST /integrations/gitflame/issues/analyze<br/>(issue, .ai.yml, repo context)
    BE->>BE: Validate .ai.yml
    alt .ai.yml missing or invalid
        BE-->>GF: Error (config required)
        GF-->>User: Show config hint / open Work with AI
    else config OK
        BE->>ML: Send issue + context
        ML-->>BE: plan.md (Markdown)
        BE-->>GF: status=plan_generated + plan_markdown + comment_body
        GF-->>User: Show plan for review

        alt User approves (/approve)
            User->>GF: Approve
            GF->>BE: POST /ai/issues/{id}/approve
            BE->>Git: Create branch + commit files + open PR
            Git-->>BE: branch_name, pull_request_url, reviewer
            BE-->>GF: status=approved + git_workflow
            GF-->>User: Show branch + PR link
        else User requests changes (/correct)
            User->>GF: Correction feedback
            GF->>BE: POST /ai/issues/{id}/correct (feedback)
            BE->>ML: Regenerate with feedback
            ML-->>BE: updated plan.md
            BE-->>GF: status=correction_requested + new plan
            GF-->>User: Show revised plan
        else User rejects (/reject)
            User->>GF: Reject
            GF->>BE: POST /ai/issues/{id}/reject
            BE-->>GF: status=rejected
            GF-->>User: Plan closed
        end
    end
```

## Plan states

```mermaid
stateDiagram-v2
    [*] --> plan_generated: analyze issue
    plan_generated --> approved: /approve
    plan_generated --> correction_requested: /correct
    plan_generated --> rejected: /reject
    correction_requested --> plan_generated: regenerate
    approved --> [*]: branch + PR created (mock)
    rejected --> [*]
```

Frontend mapping: this flow is implemented in
`frontend/src/components/IssuePlanPanel.vue`, opened from the **Work with AI**
button on the repository Code page.

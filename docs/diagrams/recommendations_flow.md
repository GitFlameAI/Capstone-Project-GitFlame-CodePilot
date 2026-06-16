# User flow: Repository Recommendation Analysis

This is the recommendation scenario. When a `.ai.yml` is present, the system
analyzes the repository, produces a short summary and a list of recommendation
cards, stores them, and surfaces them in the widget on the Code tab plus a
detailed analysis page.

```mermaid
sequenceDiagram
    actor User
    participant GF as GitFlame Code tab
    participant W as Recommendations Widget
    participant BE as Backend (Go)
    participant ML as ML Service (recommendations)
    participant DB as Database

    Note over GF,W: Code tab opens
    W->>BE: GET /repositories/{id}/recommendations/status
    W->>BE: GET /repositories/{id}/recommendations/summary
    W->>BE: GET /repositories/{id}/recommendations
    alt No analysis stored yet (404 / empty)
        BE-->>W: empty
        W-->>User: Empty state + "Run analysis"
        User->>W: Run analysis
        W->>BE: POST .../recommendations/analyze (.ai.yml + context)
        BE->>ML: Analyze repository
        ML-->>BE: summary + recommendation cards
        BE->>DB: Store summary + cards
        BE-->>W: summary + cards
    else Analysis exists
        BE-->>W: summary + cards
    end
    W-->>User: Summary + top recommendation cards

    User->>W: View detailed analysis
    W-->>User: Detailed page (filters by severity)

    alt Resolve a recommendation
        User->>BE: PATCH /recommendations/{id}/close
        BE->>DB: state = closed
        BE-->>User: Updated card (resolved)
    else Dismiss a recommendation
        User->>BE: DELETE /recommendations/{id}
        BE->>DB: remove
        BE-->>User: 204 (card removed)
    end
```

## Recommendation card states

```mermaid
stateDiagram-v2
    [*] --> open: analysis produces card
    open --> closed: PATCH /close (resolve)
    open --> [*]: DELETE (dismiss)
    closed --> [*]: DELETE (dismiss)
```

## Recommendation card shape

Each card carries: `id`, `severity` (low | medium | high), `file`, optional
`line`, `problem`, `suggestion`, optional `confidence`, `state` (open | closed),
and an optional `category` (code_duplication | security | maintainability |
performance | architecture).

Frontend mapping: the widget is `frontend/src/components/RecommendationsWidget.vue`
(Code tab), and the full report is `frontend/src/views/DetailedAnalysisView.vue`
(route `/recommendations`).

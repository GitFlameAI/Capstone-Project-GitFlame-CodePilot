# Manual Evaluation Summary

Manual evaluation was performed for `issue_02`, because full model outputs were generated for this smoke benchmark issue.
Other issues remain prepared for future evaluation but are not scored yet.

## Issue

`issue_02`: Make ML client resilient to transient failures.

Expected relevant files:

- `backend/internal/app/services.go`
- `backend/internal/app/config.go`

## Scores

| Model | Format | File relevance | Completeness | Hallucinations | Feasibility | Tests quality | Summary |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Poolside Laguna XS.2 | 1 | 5 | 5 | 5 | 5 | 5 | Best strict-format answer together with Devstral; complete and concrete. |
| Cohere North Mini Code | 0 | 5 | 4 | 4 | 4 | 5 | Strong reasoning and tests, but not a strict `plan.md`. |
| Devstral Small 2 | 1 | 5 | 4 | 5 | 4 | 4 | Fastest valid answer; slightly less detailed than Laguna/Qwen. |
| Mellum2 12B Thinking | 0 | 5 | 3 | 4 | 3 | 2 | Useful reasoning, but Thinking output prevents clean plan evaluation. |
| Qwen3.6-35B-A3B | 0 | 5 | 5 | 5 | 5 | 5 | Very complete plan, but invalid format because it starts with reasoning before the required sections. |

## Interpretation

`Devstral Small 2` is the best practical candidate in the container smoke benchmark because it produced a valid plan with the lowest latency and moderate VRAM usage.
`Laguna XS.2` produced the strongest strict-format plan, but required significantly more VRAM.
`Qwen3.6-35B-A3B` produced a high-quality plan, but needs stricter output control.
`North Mini Code` and `Mellum2 Thinking` also retrieved the right files, but require prompt/model-specific tuning before they can be used as strict plan generators.

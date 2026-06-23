# Real Model vs. Sprint 1 Mock Baseline

The same issue was used for both outputs: **Make ML client resilient to transient failures**.

| Criterion | Sprint 1 mock baseline | Qwen3-Coder real model | Result |
|---|---|---|---|
| Structure | 2 sections: Issue and Steps | All 9 required `plan.md` sections | Real model follows the approved format |
| Completeness | 5 generic workflow steps | Goal, proposed changes, 8 implementation steps, tests, risks, and open questions | Real plan covers implementation and verification |
| Relevant files | No file paths | 3 supplied files: `services.go`, `config.go`, and `server.go` | Real plan connects the issue to repository context |
| Technical specificity | Generic repository and PR workflow | Identifies `MLClient`, `postJSON`, retries, timeout configuration, HTTP 429/5xx, cancellation, and error sanitization | Real plan is specific to the issue |
| Practical usefulness | Demonstrates API/UI workflow only | Can guide implementation and review | Real output is usable after human approval |

## Conclusion

The Sprint 1 mock proved the issue-to-plan workflow but did not analyze the issue or repository. The real model produced a structured, repository-aware plan with concrete implementation and verification steps. It therefore expands Version 2 from a workflow prototype into a usable AI-assisted planning flow.

The real plan still requires human review: default retry count, backoff strategy, and configuration ownership remain open decisions. This matches the intended approve/correct/reject workflow.

## Evidence

- Sprint 1 implementation: `backend/internal/app/services.go`, function `fallbackIssuePlan`.
- Rendered mock output: `context_AI/ml/demo/version2/mock_baseline.md`.
- Unedited model output: `context_AI/ml/demo/version2/plan.md`.
- Generation metadata: `context_AI/ml/demo/version2/metadata.json`.

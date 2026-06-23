from fastapi import FastAPI

from app.schemas import IssuePlanRequest, IssuePlanResponse, RecommendationRequest, RecommendationResponse


app = FastAPI(
    title="GitFlame CodePilot ML Service",
    description="Mock ML service for issue planning, code generation, and recommendations.",
    version="0.1.0",
)


@app.get("/health")
def health_check() -> dict[str, str]:
    return {"status": "ok", "service": "ml_service"}


@app.post("/issue-plan", response_model=IssuePlanResponse)
def generate_issue_plan(payload: IssuePlanRequest) -> IssuePlanResponse:
    return IssuePlanResponse(
        plan_markdown=(
            "# Implementation Plan\n\n"
            "1. Understand the issue.\n"
            "2. Retrieve relevant repository context.\n"
            "3. Generate implementation steps.\n"
        )
    )


@app.post("/recommendations", response_model=RecommendationResponse)
def generate_recommendations(payload: RecommendationRequest) -> RecommendationResponse:
    return RecommendationResponse(
        summary="No critical issues detected in the Sprint 1 mock response.",
        recommendations=[],
    )


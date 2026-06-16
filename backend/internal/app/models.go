package app

type RepositoryMetadata struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	DefaultBranch string `json:"default_branch"`
	WebURL        string `json:"web_url,omitempty"`
}

type IssuePayload struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Author string `json:"author"`
}

type IssueAnalyzeRequest struct {
	Repository        RepositoryMetadata `json:"repository"`
	Issue             IssuePayload       `json:"issue"`
	YAMLConfig        string             `json:"yaml_config"`
	RepositoryContext []string           `json:"repository_context"`
	Metadata          map[string]any     `json:"metadata,omitempty"`
}

type GitWorkflowResponse struct {
	BranchName     string `json:"branch_name"`
	PullRequestURL string `json:"pull_request_url"`
	Reviewer       string `json:"reviewer"`
	Provider       string `json:"provider"`
}

type IssueAnalyzeResponse struct {
	SessionID    string            `json:"session_id"`
	IssueID      string            `json:"issue_id"`
	RepositoryID string            `json:"repository_id"`
	Status       string            `json:"status"`
	PlanMarkdown string            `json:"plan_markdown"`
	CommentBody  string            `json:"comment_body"`
	NextActions  map[string]string `json:"next_actions"`
}

type IssuePlanResponse struct {
	SessionID    string `json:"session_id"`
	IssueID      string `json:"issue_id"`
	RepositoryID string `json:"repository_id"`
	Status       string `json:"status"`
	PlanMarkdown string `json:"plan_markdown"`
	CommentBody  string `json:"comment_body"`
	Revision     int    `json:"revision"`
}

type PlanCorrectionRequest struct {
	Feedback string `json:"feedback"`
}

type PlanActionResponse struct {
	SessionID    string               `json:"session_id"`
	IssueID      string               `json:"issue_id"`
	Status       string               `json:"status"`
	Message      string               `json:"message"`
	PlanMarkdown string               `json:"plan_markdown,omitempty"`
	GitWorkflow  *GitWorkflowResponse `json:"git_workflow,omitempty"`
}

type RecommendationAnalyzeRequest struct {
	Repository        RepositoryMetadata `json:"repository"`
	YAMLConfig        string             `json:"yaml_config"`
	RepositoryContext []string           `json:"repository_context"`
}

type RecommendationCard struct {
	ID         string   `json:"id"`
	Severity   string   `json:"severity"`
	File       string   `json:"file"`
	Line       *int     `json:"line,omitempty"`
	Problem    string   `json:"problem"`
	Suggestion string   `json:"suggestion"`
	Confidence *float64 `json:"confidence,omitempty"`
	State      string   `json:"state"`
}

type RecommendationAnalyzeResponse struct {
	RepositoryID    string               `json:"repository_id"`
	Status          string               `json:"status"`
	Summary         string               `json:"summary"`
	Recommendations []RecommendationCard `json:"recommendations"`
}

type RecommendationStatusResponse struct {
	RepositoryID string `json:"repository_id"`
	Status       string `json:"status"`
	Total        int    `json:"total"`
	Open         int    `json:"open"`
	Closed       int    `json:"closed"`
}

type RecommendationSummaryResponse struct {
	RepositoryID string `json:"repository_id"`
	Summary      string `json:"summary"`
}

type RecommendationListResponse struct {
	RepositoryID    string               `json:"repository_id"`
	Recommendations []RecommendationCard `json:"recommendations"`
}

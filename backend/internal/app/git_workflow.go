package app

import (
	"regexp"
	"strings"
)

type GitWorkflowService interface {
	CreatePullRequest(request GitWorkflowContractRequest) (GitWorkflowContract, error)
}

type MockGitWorkflowService struct{}

func NewMockGitWorkflowService() *MockGitWorkflowService {
	return &MockGitWorkflowService{}
}

type GitWorkflowContractRequest struct {
	IssueRequest IssueAnalyzeRequest
	Config       AIConfig
	PlanMarkdown string
}

type GitWorkflowContract struct {
	BranchCreation     BranchCreationPayload     `json:"branch_creation"`
	GeneratedFiles     GeneratedFilesPayload     `json:"generated_files"`
	PullRequest        PullRequestPayload        `json:"pull_request"`
	ReviewerAssignment ReviewerAssignmentPayload `json:"reviewer_assignment"`
	Response           GitWorkflowResponse       `json:"response"`
}

type BranchCreationPayload struct {
	RepositoryID string `json:"repository_id"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
}

type GeneratedFilesPayload struct {
	RepositoryID string               `json:"repository_id"`
	BranchName   string               `json:"branch_name"`
	Files        []GeneratedFilePatch `json:"files"`
}

type GeneratedFilePatch struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Content   string `json:"content"`
}

type PullRequestPayload struct {
	RepositoryID string `json:"repository_id"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	Title        string `json:"title"`
	Body         string `json:"body"`
}

type ReviewerAssignmentPayload struct {
	RepositoryID string `json:"repository_id"`
	PullRequest  string `json:"pull_request"`
	Reviewer     string `json:"reviewer"`
	Policy       string `json:"policy"`
}

func (s *MockGitWorkflowService) CreatePullRequest(contractRequest GitWorkflowContractRequest) (GitWorkflowContract, error) {
	request := contractRequest.IssueRequest
	cfg := contractRequest.Config
	slug := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(request.Issue.Title, "-")
	slug = strings.Trim(strings.ToLower(slug), "-")
	if len(slug) > 48 {
		slug = slug[:48]
	}
	if slug == "" {
		slug = strings.ToLower(request.Issue.ID)
	}

	baseURL := request.Repository.WebURL
	if baseURL == "" {
		baseURL = "https://gitflame.local/" + request.Repository.ID
	}

	reviewer := request.Issue.Author
	if cfg.ReviewerPolicy != "issue_author" {
		reviewer = cfg.ReviewerPolicy
	}

	branchName := cfg.TargetBranchPrefix + request.Issue.ID + "-" + slug
	prURL := strings.TrimRight(baseURL, "/") + "/-/merge_requests/mock-" + request.Issue.ID
	response := GitWorkflowResponse{
		BranchName:     branchName,
		PullRequestURL: prURL,
		Reviewer:       reviewer,
		Provider:       "mock",
	}

	return GitWorkflowContract{
		BranchCreation: BranchCreationPayload{
			RepositoryID: request.Repository.ID,
			SourceBranch: cfg.DefaultBranch,
			TargetBranch: branchName,
		},
		GeneratedFiles: GeneratedFilesPayload{
			RepositoryID: request.Repository.ID,
			BranchName:   branchName,
			Files: []GeneratedFilePatch{
				{
					Path:      "AI_PLAN.md",
					Operation: "upsert",
					Content:   contractRequest.PlanMarkdown,
				},
			},
		},
		PullRequest: PullRequestPayload{
			RepositoryID: request.Repository.ID,
			SourceBranch: branchName,
			TargetBranch: cfg.DefaultBranch,
			Title:        request.Issue.Title,
			Body:         "Mock pull request generated from approved AI implementation plan.",
		},
		ReviewerAssignment: ReviewerAssignmentPayload{
			RepositoryID: request.Repository.ID,
			PullRequest:  prURL,
			Reviewer:     reviewer,
			Policy:       cfg.ReviewerPolicy,
		},
		Response: response,
	}, nil
}

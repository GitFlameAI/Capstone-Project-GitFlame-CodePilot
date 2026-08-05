package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitflame-codepilot/backend/internal/domain"
	"gitflame-codepilot/backend/internal/repository"
	"gitflame-codepilot/backend/internal/service"
)

const maxWebhookBodyBytes = 2 << 20

type webhookRegistrationResponse struct {
	ID           string    `json:"id"`
	ConnectionID string    `json:"connection_id"`
	WebhookURL   string    `json:"webhook_url"`
	Secret       string    `json:"secret,omitempty"`
	Events       []string  `json:"events"`
	Status       string    `json:"status"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Server) createGitFlameWebhook(w http.ResponseWriter, r *http.Request) {
	connection, err := s.authenticatedConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "webhook_connection_error")
		return
	}
	if s.publicBaseURL == "" {
		problem(w, http.StatusUnprocessableEntity, "public_base_url_required", "PUBLIC_BASE_URL must be the externally reachable CodePilot API base URL before enabling a webhook")
		return
	}
	registrationID := repository.NewID()
	if existing, lookupErr := s.store.GitFlameWebhookByConnection(connection.ID); lookupErr == nil {
		registrationID = existing.ID
	} else if !errors.Is(lookupErr, repository.ErrNotFound) {
		problem(w, http.StatusInternalServerError, "storage_error", lookupErr.Error())
		return
	}
	secret, secretHash, err := newWebhookSecret(s.webhookSigningKey[:], registrationID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "secret_generation_error", err.Error())
		return
	}
	webhookURL := s.publicBaseURL + "/integrations/gitflame/webhooks/" + registrationID
	saved, err := s.store.SaveGitFlameWebhook(domain.GitFlameWebhookRegistration{
		ID: registrationID, ConnectionID: connection.ID, WebhookURL: webhookURL,
		WebhookSecretHash: secretHash, Events: []string{"push", "issues"}, Status: "active",
	})
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	write(w, http.StatusCreated, webhookResponse(saved, secret))
}

func (s *Server) getGitFlameWebhook(w http.ResponseWriter, r *http.Request) {
	connection, err := s.authenticatedConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "webhook_connection_error")
		return
	}
	webhook, err := s.store.GitFlameWebhookByConnection(connection.ID)
	if err != nil {
		resourceError(w, err, "webhook_not_found", "webhook is not enabled for this connection")
		return
	}
	write(w, http.StatusOK, webhookResponse(webhook, ""))
}

func (s *Server) disableGitFlameWebhook(w http.ResponseWriter, r *http.Request) {
	connection, err := s.authenticatedConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "webhook_connection_error")
		return
	}
	webhook, err := s.store.GitFlameWebhookByConnection(connection.ID)
	if err != nil {
		resourceError(w, err, "webhook_not_found", "webhook is not enabled for this connection")
		return
	}
	webhook.Status = "disabled"
	saved, err := s.store.SaveGitFlameWebhook(*webhook)
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	write(w, http.StatusOK, webhookResponse(saved, ""))
}

func (s *Server) listGitFlameWebhookEvents(w http.ResponseWriter, r *http.Request) {
	connection, err := s.authenticatedConnection(r, r.PathValue("id"))
	if err != nil {
		integrationError(w, err, "webhook_connection_error")
		return
	}
	webhook, err := s.store.GitFlameWebhookByConnection(connection.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			write(w, http.StatusOK, map[string]any{"events": []domain.GitFlameWebhookEvent{}})
			return
		}
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.store.GitFlameWebhookEvents(webhook.ID, limit)
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"webhook_id": webhook.ID, "events": events})
}

func (s *Server) receiveGitFlameWebhook(w http.ResponseWriter, r *http.Request) {
	webhook, err := s.store.GitFlameWebhook(r.PathValue("id"))
	if err != nil || webhook.Status != "active" {
		problem(w, http.StatusNotFound, "webhook_not_found", "active webhook was not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		problem(w, http.StatusRequestEntityTooLarge, "webhook_payload_too_large", "webhook payload exceeds 2 MiB")
		return
	}
	if !validWebhookRequest(s.webhookSigningKey[:], webhook, r, body) {
		problem(w, http.StatusUnauthorized, "invalid_webhook_secret", "webhook secret or signature is missing or invalid")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		problem(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	eventType := normalizedWebhookEvent(r, payload)
	action := firstWebhookString(payload, "action", "issue.action", "issue.state", "state")
	deliveryID := firstNonEmptyHeader(r, "X-GitFlame-Delivery", "X-Gitea-Delivery", "X-GitHub-Delivery")
	if deliveryID != "" {
		existing, listErr := s.store.GitFlameWebhookEvents(webhook.ID, 100)
		if listErr == nil {
			for _, event := range existing {
				if event.DeliveryID == deliveryID {
					write(w, http.StatusAccepted, map[string]any{"status": "duplicate", "event_id": event.ID})
					return
				}
			}
		}
	}
	connection, err := s.store.GitFlameConnection(webhook.ConnectionID)
	if err != nil {
		problem(w, http.StatusGone, "connection_not_found", "webhook connection no longer exists")
		return
	}
	repositoryID := firstWebhookString(payload, "repository.full_name", "repository.path_with_namespace", "repository.id")
	event := domain.GitFlameWebhookEvent{
		WebhookID: webhook.ID, EventType: eventType, Action: action, DeliveryID: deliveryID,
		RepositoryID: repositoryID, Payload: payload, Status: "received",
	}
	saved, err := s.store.SaveGitFlameWebhookEvent(event)
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", err.Error())
		return
	}
	log.Printf("gitflame_webhook event_id=%s type=%s action=%s connection_id=%s", saved.ID, eventType, action, connection.ID)
	write(w, http.StatusAccepted, map[string]any{"status": "received", "event_id": saved.ID})
	go s.processGitFlameWebhookEvent(*webhook, *connection, *saved)
}

func (s *Server) processGitFlameWebhookEvent(_ domain.GitFlameWebhookRegistration, connection domain.GitFlameConnection, event domain.GitFlameWebhookEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var err error
	switch event.EventType {
	case "push":
		err = s.processRepositoryChange(ctx, &connection, &event)
	case "issue":
		// Issue content and status are refreshed by the authenticated frontend poll.
		event.Payload["issues_refresh_required"] = true
	default:
		event.Status = "ignored"
	}
	finished := time.Now().UTC()
	event.ProcessedAt = &finished
	if err != nil {
		event.Status = "failed"
		event.Error = &domain.TaskError{HTTPStatus: http.StatusBadGateway, Code: "webhook_processing_failed", Detail: err.Error()}
		log.Printf("gitflame_webhook event_id=%s status=failed error=%q", event.ID, err)
	} else if event.Status == "received" {
		event.Status = "processed"
	}
	if _, saveErr := s.store.SaveGitFlameWebhookEvent(event); saveErr != nil {
		log.Printf("gitflame_webhook event_id=%s status_update_error=%q", event.ID, saveErr)
	}
}

func (s *Server) processRepositoryChange(ctx context.Context, connection *domain.GitFlameConnection, event *domain.GitFlameWebhookEvent) error {
	reader, err := s.gitFlameReaderFromStoredConnection(connection)
	if err != nil {
		return err
	}
	ref := firstWebhookString(event.Payload, "ref", "repository.default_branch")
	if strings.HasPrefix(ref, "refs/heads/") {
		ref = strings.TrimPrefix(ref, "refs/heads/")
	}
	if ref == "" {
		ref = connection.DefaultBranch
	}
	commitSHA := firstWebhookString(event.Payload, "after", "commit_sha", "head_commit.id")
	syncResult, err := s.synchronizeRepositoryIndex(ctx, reader, connection, ref, commitSHA, false)
	if err != nil {
		return err
	}
	yamlConfig, files := syncResult.YAMLConfig, syncResult.Files
	if files == nil {
		yamlConfig, files, err = repositoryFilesForAnalysis(
			ctx, reader, connection.Repository.ID, ref, yamlConfig, nil,
		)
		if err != nil {
			return err
		}
	}
	event.Payload["tree_refresh_required"] = true
	event.Payload["rag_index_status"] = syncResult.Result.Status
	event.Payload["rag_commit_sha"] = syncResult.Result.CommitSHA
	event.Payload["recommendations_status"] = "skipped"
	cfg, err := service.ParseAIConfig(yamlConfig)
	if err != nil {
		return fmt.Errorf("cannot analyze recommendations after push: %w", err)
	}
	if !cfg.RecommendationsEnabled {
		_, err = s.store.SaveRecommendations(connection.Repository, cfg, "Recommendation analysis is disabled by .ai.yml categories.", []domain.RecommendationCard{})
		return err
	}
	if err := service.ValidateRepositoryFilesForIntegration(files); err != nil {
		return err
	}
	if s.recommender == nil {
		return errors.New("recommendation service is not configured")
	}
	connection.Repository.CommitSHA = syncResult.Result.CommitSHA
	summary, cards, err := s.recommender.AnalyzeRecommendations(
		ctx, connection.Repository, yamlConfig, files,
	)
	if err != nil {
		return err
	}
	for index := range cards {
		if cards[index].ID == "" {
			cards[index].ID = repository.NewID()
		}
		if cards[index].State == "" {
			cards[index].State = "open"
		}
		if cards[index].Severity == "" {
			cards[index].Severity = "medium"
		}
	}
	if _, err := s.store.SaveRecommendations(connection.Repository, cfg, summary, cards); err != nil {
		return err
	}
	event.Payload["recommendations_status"] = "updated"
	_ = s.store.TouchGitFlameConnection(connection.UserID, connection.ID)
	return nil
}

func (s *Server) authenticatedConnection(r *http.Request, connectionID string) (*domain.GitFlameConnection, error) {
	session, err := s.authenticate(r)
	if err != nil {
		return nil, &IntegrationError{Status: http.StatusUnauthorized, Code: "unauthorized", Detail: "valid application session cookie is required"}
	}
	connection, err := s.store.UserGitFlameConnection(session.User.ID, connectionID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, &IntegrationError{Status: http.StatusNotFound, Code: "connection_not_found", Detail: "GitFlame connection was not found"}
	}
	return connection, err
}

func webhookResponse(webhook *domain.GitFlameWebhookRegistration, secret string) webhookRegistrationResponse {
	return webhookRegistrationResponse{
		ID: webhook.ID, ConnectionID: webhook.ConnectionID, WebhookURL: webhook.WebhookURL,
		Secret: secret, Events: append([]string(nil), webhook.Events...), Status: webhook.Status, UpdatedAt: webhook.UpdatedAt,
	}
}

func newWebhookSecret(masterKey []byte, webhookID string) (string, string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", "", err
	}
	salt := base64.RawURLEncoding.EncodeToString(saltBytes)
	secret := deriveWebhookSecret(masterKey, webhookID, salt)
	digest := sha256.Sum256([]byte(secret))
	return secret, "v1:" + salt + ":" + hex.EncodeToString(digest[:]), nil
}

func validWebhookSecret(expectedHash, secret string) bool {
	hash := expectedHash
	if parts := strings.Split(expectedHash, ":"); len(parts) == 3 && parts[0] == "v1" {
		hash = parts[2]
	}
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != sha256.Size || secret == "" {
		return false
	}
	digest := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(decoded, digest[:]) == 1
}

func validWebhookRequest(masterKey []byte, webhook *domain.GitFlameWebhookRegistration, r *http.Request, body []byte) bool {
	if validWebhookSecret(webhook.WebhookSecretHash, webhookSecretFromRequest(r)) {
		return true
	}
	parts := strings.Split(webhook.WebhookSecretHash, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return false
	}
	secret := deriveWebhookSecret(masterKey, webhook.ID, parts[1])
	for _, header := range []string{"X-Gitea-Signature", "X-Hub-Signature-256"} {
		signature := strings.TrimSpace(r.Header.Get(header))
		signature = strings.TrimPrefix(signature, "sha256=")
		provided, err := hex.DecodeString(signature)
		if err != nil || len(provided) != sha256.Size {
			continue
		}
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		if hmac.Equal(provided, mac.Sum(nil)) {
			return true
		}
	}
	return false
}

func deriveWebhookSecret(masterKey []byte, webhookID, salt string) string {
	mac := hmac.New(sha256.New, masterKey)
	_, _ = mac.Write([]byte(webhookID + ":" + salt))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func webhookSecretFromRequest(r *http.Request) string {
	secret := firstNonEmptyHeader(r, "X-GitFlame-Webhook-Secret", "X-GitFlame-Token", "X-Gitlab-Token")
	if secret != "" {
		return secret
	}
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		return strings.TrimSpace(authorization[7:])
	}
	return ""
}

func normalizedWebhookEvent(r *http.Request, payload map[string]any) string {
	raw := strings.ToLower(firstNonEmptyHeader(r, "X-GitFlame-Event", "X-Gitea-Event", "X-GitHub-Event"))
	if raw == "" {
		raw = strings.ToLower(firstWebhookString(payload, "event", "event_type", "object_kind", "type"))
	}
	if strings.Contains(raw, "issue") || firstWebhookValue(payload, "issue") != nil {
		return "issue"
	}
	if strings.Contains(raw, "push") || strings.Contains(raw, "commit") || strings.Contains(raw, "repository") {
		return "push"
	}
	return "unknown"
}

func firstNonEmptyHeader(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func firstWebhookString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		if value := firstWebhookValue(payload, path); value != nil {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return strings.TrimSpace(typed)
				}
			case json.Number:
				return typed.String()
			case float64:
				return strconv.FormatFloat(typed, 'f', -1, 64)
			}
		}
	}
	return ""
}

func firstWebhookValue(payload map[string]any, path string) any {
	var current any = payload
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}

package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr, AgentEngineURL, RedisURL, DatabaseURL          string
	GitFlameBaseURL                                      string
	GitFlameCredentialKey                                string
	PublicBaseURL                                        string
	RAGBaseURL, RAGAPIKey                                string
	SessionCookieName                                    string
	LogLevel, LogFormat                                  string
	WorkerMetricsAddr                                    string
	RecommendationServiceURL                             string
	AgentQueueName, AgentConsumerGroup, DispatchMode     string
	AgentTimeout, GitFlameTimeout, RecommendationTimeout time.Duration
	RAGIndexTimeout, RAGIndexWaitTimeout                 time.Duration
	QueueMaxLength, WorkerMaxRetries                     int
	GitFlameCredentialKeyVersion                         int
	SessionTTL                                           time.Duration
	SessionCookieSecure                                  bool
}

func Load() Config {
	seconds, err := strconv.Atoi(env("AGENT_ENGINE_TIMEOUT_SECONDS", "120"))
	if err != nil || seconds < 1 {
		seconds = 120
	}
	gitFlameSeconds := positiveInt("GITFLAME_TIMEOUT_SECONDS", 30)
	recommendationSeconds := positiveInt("RECOMMENDATION_SERVICE_TIMEOUT_SECONDS", 120)
	ragIndexSeconds := positiveInt("RAG_INDEX_TIMEOUT_SECONDS", 1800)
	ragIndexWaitSeconds := positiveInt("RAG_INDEX_WAIT_TIMEOUT_SECONDS", ragIndexSeconds)
	queueMaxLength := positiveInt("AGENT_QUEUE_MAX_LENGTH", 1000)
	workerMaxRetries := positiveInt("WORKER_MAX_RETRIES", 3)
	sessionTTLHours := positiveInt("SESSION_TTL_HOURS", 168)
	return Config{
		Addr:                         ":" + env("BACKEND_PORT", "8000"),
		AgentEngineURL:               env("AGENT_ENGINE_URL", "http://localhost:8001"),
		RedisURL:                     env("REDIS_URL", ""),
		DatabaseURL:                  env("DATABASE_URL", ""),
		GitFlameBaseURL:              env("GITFLAME_BASE_URL", ""),
		GitFlameCredentialKey:        env("GITFLAME_CREDENTIAL_KEY", ""),
		PublicBaseURL:                strings.TrimRight(env("PUBLIC_BASE_URL", ""), "/"),
		RAGBaseURL:                   env("RAG_BASE_URL", ""),
		RAGAPIKey:                    env("RAG_API_KEY", ""),
		SessionCookieName:            env("SESSION_COOKIE_NAME", "codepilot_session"),
		LogLevel:                     env("LOG_LEVEL", "info"),
		WorkerMetricsAddr:            ":" + env("WORKER_METRICS_PORT", "9100"),
		LogFormat:                    env("LOG_FORMAT", "json"),
		RecommendationServiceURL:     env("RECOMMENDATION_SERVICE_URL", ""),
		AgentQueueName:               env("AGENT_QUEUE_NAME", "gitflame:agent:tasks"),
		AgentConsumerGroup:           env("AGENT_CONSUMER_GROUP", "gitflame-agent-workers"),
		DispatchMode:                 env("TASK_DISPATCH_MODE", "local"),
		AgentTimeout:                 time.Duration(seconds) * time.Second,
		GitFlameTimeout:              time.Duration(gitFlameSeconds) * time.Second,
		RecommendationTimeout:        time.Duration(recommendationSeconds) * time.Second,
		RAGIndexTimeout:              time.Duration(ragIndexSeconds) * time.Second,
		RAGIndexWaitTimeout:          time.Duration(ragIndexWaitSeconds) * time.Second,
		QueueMaxLength:               queueMaxLength,
		WorkerMaxRetries:             workerMaxRetries,
		GitFlameCredentialKeyVersion: positiveInt("GITFLAME_CREDENTIAL_KEY_VERSION", 1),
		SessionTTL:                   time.Duration(sessionTTLHours) * time.Hour,
		SessionCookieSecure:          boolEnv("SESSION_COOKIE_SECURE", false),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func positiveInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

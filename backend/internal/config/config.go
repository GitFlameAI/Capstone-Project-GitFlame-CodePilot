package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr, AgentEngineURL, RedisURL, DatabaseURL          string
	GitFlameBaseURL, GitFlameAPIKey                      string
	RecommendationServiceURL                             string
	AgentQueueName, AgentConsumerGroup, DispatchMode     string
	AgentTimeout, GitFlameTimeout, RecommendationTimeout time.Duration
	QueueMaxLength, WorkerMaxRetries                     int
}

func Load() Config {
	seconds, err := strconv.Atoi(env("AGENT_ENGINE_TIMEOUT_SECONDS", "120"))
	if err != nil || seconds < 1 {
		seconds = 120
	}
	gitFlameSeconds := positiveInt("GITFLAME_TIMEOUT_SECONDS", 30)
	recommendationSeconds := positiveInt("RECOMMENDATION_SERVICE_TIMEOUT_SECONDS", 120)
	queueMaxLength := positiveInt("AGENT_QUEUE_MAX_LENGTH", 1000)
	workerMaxRetries := positiveInt("WORKER_MAX_RETRIES", 3)
	return Config{
		Addr:                     ":" + env("BACKEND_PORT", "8000"),
		AgentEngineURL:           env("AGENT_ENGINE_URL", env("ML_SERVICE_URL", "http://localhost:8001")),
		RedisURL:                 env("REDIS_URL", ""),
		DatabaseURL:              env("DATABASE_URL", ""),
		GitFlameBaseURL:          env("GITFLAME_BASE_URL", ""),
		GitFlameAPIKey:           env("GITFLAME_API_KEY", ""),
		RecommendationServiceURL: env("RECOMMENDATION_SERVICE_URL", ""),
		AgentQueueName:           env("AGENT_QUEUE_NAME", "gitflame:agent:tasks"),
		AgentConsumerGroup:       env("AGENT_CONSUMER_GROUP", "gitflame-agent-workers"),
		DispatchMode:             env("TASK_DISPATCH_MODE", "local"),
		AgentTimeout:             time.Duration(seconds) * time.Second,
		GitFlameTimeout:          time.Duration(gitFlameSeconds) * time.Second,
		RecommendationTimeout:    time.Duration(recommendationSeconds) * time.Second,
		QueueMaxLength:           queueMaxLength,
		WorkerMaxRetries:         workerMaxRetries,
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

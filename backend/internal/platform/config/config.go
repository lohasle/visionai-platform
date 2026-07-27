package config

import (
	"os"
	"time"
)

type Config struct {
	HTTPAddr           string
	DBDSN              string
	RabbitMQURL        string
	EventExchange      string
	EventQueue         string
	S3Endpoint         string
	S3PublicEndpoint   string
	S3AccessKey        string
	S3SecretKey        string
	S3Bucket           string
	S3Secure           bool
	ImportRoot         string
	CVATBaseURL        string
	CVATPublicURL      string
	CVATUsername       string
	CVATPassword       string
	CVATTimeout        time.Duration
	TrainingWorkRoot   string
	DockerBinary       string
	DockerVolumesFrom  string
	DockerNetwork      string
	TrainingTimeout    time.Duration
	ClearMLAPIURL      string
	ClearMLWebURL      string
	ClearMLAccessKey   string
	ClearMLSecretKey   string
	FiftyOneAPIURL     string
	FiftyOnePublicURL  string
	FiftyOneMediaHost  string
	FiftyOneTimeout    time.Duration
	InferenceAPIURL    string
	InferencePublicURL string
	InferenceTimeout   time.Duration
	JWTSecret          string
	TokenTTL           time.Duration
	RefreshTokenTTL    time.Duration
	BootstrapTenant    string
	BootstrapUser      string
	BootstrapPass      string
}

func Load() Config {
	return Config{
		HTTPAddr:           env("NIMBUS_HTTP_ADDR", ":58080"),
		DBDSN:              env("NIMBUS_DB_DSN", "nimbus:nimbus_dev@tcp(127.0.0.1:23316)/nimbus_platform_go?charset=utf8mb4&parseTime=True&loc=Local"),
		RabbitMQURL:        env("NIMBUS_RABBITMQ_URL", "amqp://visionai:visionai_dev@127.0.0.1:25672/"),
		EventExchange:      env("NIMBUS_EVENT_EXCHANGE", "visionai.events"),
		EventQueue:         env("NIMBUS_EVENT_QUEUE", "visionai.domain-events.v1"),
		S3Endpoint:         env("NIMBUS_S3_ENDPOINT", "127.0.0.1:29000"),
		S3PublicEndpoint:   env("NIMBUS_S3_PUBLIC_ENDPOINT", ""),
		S3AccessKey:        env("NIMBUS_S3_ACCESS_KEY", "visionai"),
		S3SecretKey:        env("NIMBUS_S3_SECRET_KEY", "visionai_minio_dev"),
		S3Bucket:           env("NIMBUS_S3_BUCKET", "visionai-assets"),
		S3Secure:           boolean("NIMBUS_S3_SECURE", false),
		ImportRoot:         env("NIMBUS_IMPORT_ROOT", "/imports"),
		CVATBaseURL:        env("NIMBUS_CVAT_BASE_URL", "http://127.0.0.1:28080"),
		CVATPublicURL:      env("NIMBUS_CVAT_PUBLIC_URL", "http://127.0.0.1:28080"),
		CVATUsername:       env("NIMBUS_CVAT_USERNAME", "visionai"),
		CVATPassword:       env("NIMBUS_CVAT_PASSWORD", "visionai_cvat_dev"),
		CVATTimeout:        duration("NIMBUS_CVAT_TIMEOUT", 2*time.Minute),
		TrainingWorkRoot:   env("NIMBUS_TRAINING_WORK_ROOT", "/training-work"),
		DockerBinary:       env("NIMBUS_DOCKER_BINARY", "docker"),
		DockerVolumesFrom:  env("NIMBUS_DOCKER_VOLUMES_FROM", ""),
		DockerNetwork:      env("NIMBUS_DOCKER_NETWORK", "nimbus-framework-go_default"),
		TrainingTimeout:    duration("NIMBUS_TRAINING_TIMEOUT", 24*time.Hour),
		ClearMLAPIURL:      env("NIMBUS_CLEARML_API_URL", ""),
		ClearMLWebURL:      env("NIMBUS_CLEARML_WEB_URL", ""),
		ClearMLAccessKey:   env("NIMBUS_CLEARML_ACCESS_KEY", ""),
		ClearMLSecretKey:   env("NIMBUS_CLEARML_SECRET_KEY", ""),
		FiftyOneAPIURL:     env("NIMBUS_FIFTYONE_API_URL", "http://127.0.0.1:25152"),
		FiftyOnePublicURL:  env("NIMBUS_FIFTYONE_PUBLIC_URL", "http://127.0.0.1:25151"),
		FiftyOneMediaHost:  env("NIMBUS_FIFTYONE_MEDIA_HOST", "host.docker.internal:29000"),
		FiftyOneTimeout:    duration("NIMBUS_FIFTYONE_TIMEOUT", 5*time.Minute),
		InferenceAPIURL:    env("NIMBUS_INFERENCE_API_URL", "http://127.0.0.1:28000"),
		InferencePublicURL: env("NIMBUS_INFERENCE_PUBLIC_URL", "http://127.0.0.1:28000"),
		InferenceTimeout:   duration("NIMBUS_INFERENCE_TIMEOUT", 2*time.Minute),
		JWTSecret:          env("NIMBUS_JWT_SECRET", "nimbus-local-development-secret"),
		TokenTTL:           duration("NIMBUS_TOKEN_TTL", 2*time.Hour),
		RefreshTokenTTL:    duration("NIMBUS_REFRESH_TOKEN_TTL", 30*24*time.Hour),
		BootstrapTenant:    env("NIMBUS_BOOTSTRAP_TENANT", "Nimbus Framework"),
		BootstrapUser:      env("NIMBUS_BOOTSTRAP_USERNAME", "admin"),
		BootstrapPass:      env("NIMBUS_BOOTSTRAP_PASSWORD", "admin123"),
	}
}

func boolean(key string, fallback bool) bool {
	switch env(key, "") {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(env(key, fallback.String()))
	if err != nil {
		return fallback
	}
	return value
}

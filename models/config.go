package models

type Config struct {
	App           AppConfig           `mapstructure:",squash"`
	Server        ServerConfig        `mapstructure:",squash"`
	Database      DatabaseConfig      `mapstructure:",squash"`
	Redis         RedisConfig         `mapstructure:",squash"`
	RabbitMQ      RabbitMQConfig      `mapstructure:",squash"`
	Storage       StorageConfig       `mapstructure:",squash"`
	LLM           LLMConfig           `mapstructure:",squash"`
	Embedding     EmbeddingConfig     `mapstructure:",squash"`
	Reranker      RerankerConfig      `mapstructure:",squash"`
	Auth          AuthConfig          `mapstructure:",squash"`
	GoogleOAuth   GoogleOAuthConfig   `mapstructure:",squash"`
	Observability ObservabilityConfig `mapstructure:",squash"`
	SMTP          SMTPConfig          `mapstructure:",squash"`
	FrontEnd      FrontEnd            `mapstructure:",squash"`
}

type AppConfig struct {
	Env   string `mapstructure:"APP_ENV"`
	Batch int    `mapstructure:"OUTBOX_MESSAGES_BATCH"`
}

type ServerConfig struct {
	Port           string `mapstructure:"HTTP_PORT"`
	AllowedOrigins string `mapstructure:"ALLOWED_ORIGINS"`
}

// DatabaseConfig contains PostgreSQL configuration.
type DatabaseConfig struct {
	URL           string `mapstructure:"DATABASE_URL"`
	DSN           string `mapstructure:"DATABASE_DSN"`
	MaxOpenConns  int    `mapstructure:"MAX_OPEN_CONNS"`
	MaxIdleConns  int    `mapstructure:"MAX_IDLE_CONNS"`
	MigrationPath string `mapstructure:"MIGRATION_PATH"`
}

type RedisConfig struct {
	URL      string `mapstructure:"REDIS_URL"`
	Addr     string `mapstructure:"REDIS_ADDR"`
	Password string `mapstructure:"REDIS_PASSWORD"`
	DB       int    `mapstructure:"REDIS_DB"`
}

type RabbitMQConfig struct {
	URL      string `mapstructure:"RABBITMQ_URL"`
	User     string `mapstructure:"RABBITMQ_USER"`
	Password string `mapstructure:"RABBITMQ_PASSWORD"`
}

type StorageConfig struct {
	B2Bucket          string `mapstructure:"B2_BUCKET"`
	B2AccessKey       string `mapstructure:"B2_ACCESS_KEY"`
	B2SecretKey       string `mapstructure:"B2_SECRET_KEY"`
	B2Endpoint        string `mapstructure:"B2_ENDPOINT"`
	B2Region          string `mapstructure:"B2_REGION"`
	B2SignedURLExpiry string `mapstructure:"B2_SIGNED_URL_EXPIRY"`
}

type LLMConfig struct {
	Provider string `mapstructure:"LLM_PROVIDER"`
	APIKey   string `mapstructure:"LLM_API_KEY"`
	Model    string `mapstructure:"LLM_MODEL"`

	GeminiAPIKey          string  `mapstructure:"GEMINI_API_KEY"`
	GeminiProjectName     string  `mapstructure:"PROJECT_NAME"`
	GeminiProjectNumber   string  `mapstructure:"PROJECT_NUMBER"`
	MaxTokens             int     `mapstructure:"MAX_TOKENS"`
	Temperature           float64 `mapstructure:"TEMPERATURE"`
	InputPricePerMillion  float64 `envconfig:"LLM_INPUT_PRICE_PER_MILLION" default:"0.30"`
	OutputPricePerMillion float64 `envconfig:"LLM_OUTPUT_PRICE_PER_MILLION" default:"2.50"`
	GenerateURL           string  `mapstructure:"GENERATEURL"`
	StreamURL             string  `mapstructure:"STREAMURL"`
	MaxContextTokens      int     `mapstructure:"MAX_CONTEXT_TOKENS"`
}

type EmbeddingConfig struct {
	Provider string `mapstructure:"EMBEDDING_PROVIDER"`
	APIKey   string `mapstructure:"EMBEDDING_API_KEY"`
	Model    string `mapstructure:"EMBEDDING_MODEL"`
}

type RerankerConfig struct {
	Provider        string `mapstructure:"RERANKER_PROVIDER"`
	APIKey          string `mapstructure:"COHERE_RERANKER_API_KEY"`
	RerankerModel   string `mapstructure:"COHERE_RERANK_MODEL"`
	RerankerTimeout int    `mapstructure:"COHERE_RERANK_TIMEOUT"`
}

type AuthConfig struct {
	JWTSecret          string `mapstructure:"JWT_SECRET"`
	JWTExpirationHours int    `mapstructure:"JWT_EXPIRATION_HOURS"`

	PublicKey       string `mapstructure:"PUBLIC_KEY"`
	PrivateKey      string `mapstructure:"PRIVATE_KEY"`
	Issuer          string `mapstructure:"ISSUER"`
	AccessTokenTTL  int    `mapstructure:"ACCESS_TOKEN_TTL"`
	RefreshTokenTTL int    `mapstructure:"REFRESH_TOKEN_TTL"`
	TokenPrefix     string `mapstructure:"TOKEN_PREFIX"`
}

type GoogleOAuthConfig struct {
	ClientID     string `mapstructure:"GOOGLE_OAUTH_CLIENTID"`
	ClientSecret string `mapstructure:"GOOGLE_OAUTH_CLIENT_SECRET"`
	CallbackURL  string `mapstructure:"GOOGLE_CALLBACK_API"`
	UserInfoURL  string `mapstructure:"GOOGLE_USERINFO_API"`
}

type ObservabilityConfig struct {
	OTelEndpoint         string `mapstructure:"OTEL_ENDPOINT"`
	GrafanaAdminPassword string `mapstructure:"GRAFANA_ADMIN_PASSWORD"`
	SentryDSN            string `mapstructure:"SENTRY_DSN"`
	DataDogAPIKey        string `mapstructure:"DATADOG_API_KEY"`
}

type SMTPConfig struct {
	Host     string `mapstructure:"SMTP_HOST"`
	Port     int    `mapstructure:"SMTP_PORT"`
	Username string `mapstructure:"SMTP_USERNAME"`
	Password string `mapstructure:"SMTP_PASSWORD"`
	From     string `mapstructure:"SMTP_FROM"`
}

type FrontEnd struct {
	FrontendURL string `mapstructure:"FRONTEND_URL"`
	PrivacyURL  string `mapstructure:"PRIVACY_URL"`
	TermsURL    string `mapstructure:"TERM_URL"`
	SupportURL  string `mapstructure:"SUPPORT_URL"`
}

package config

import "github.com/caarlos0/env/v10"

// Config centraliza la configuración del servicio.
type Config struct {
	HTTPPort    string `env:"HTTP_PORT" envDefault:"8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`
	LLMAPIKey   string `env:"LLM_API_KEY,required"`
	LLMBaseURL  string `env:"LLM_BASE_URL" envDefault:"https://api.openai.com/v1"`
	LLMModel    string `env:"LLM_MODEL" envDefault:"gpt-5.1"`
	SMTPHost    string `env:"SMTP_HOST"`
	SMTPPort    int    `env:"SMTP_PORT" envDefault:"587"`
	SMTPUser    string `env:"SMTP_USER"`
	SMTPPass    string `env:"SMTP_PASS"`
	SMTPFrom    string `env:"SMTP_FROM"`
	SMTPFromName string `env:"SMTP_FROM_NAME"`
	SMTPUseTLS  bool   `env:"SMTP_USE_TLS" envDefault:"false"`
	RedisAddr   string `env:"REDIS_ADDR"`
	RedisPassword string `env:"REDIS_PASSWORD"`
	RedisDB     int    `env:"REDIS_DB" envDefault:"0"`
}

// LoadConfig carga la configuración desde variables de entorno.
func LoadConfig() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	AppPort        string
	DSN            string
	MigrationsPath string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	JWTSecret string
	JWTTTL    time.Duration

	TasksCacheTTL time.Duration

	// RateLimitRPS — сколько запросов в секунду разрешено одному клиенту (0 — без ограничения).
	RateLimitRPS   float64
	RateLimitBurst int
}

func AppConfig() (*Config, error) {
	redisDB, err := getEnvInt("REDIS_DB", 0)
	if err != nil {
		return nil, err
	}

	jwtTTL, err := getEnvDuration("JWT_TTL", 24*time.Hour)
	if err != nil {
		return nil, err
	}

	cacheTTL, err := getEnvDuration("TASKS_CACHE_TTL", 5*time.Minute)
	if err != nil {
		return nil, err
	}

	rps, err := strconv.ParseFloat(getEnv("RATE_LIMIT_RPS", "20"), 64)
	if err != nil {
		return nil, fmt.Errorf("RATE_LIMIT_RPS: %w", err)
	}

	burst, err := getEnvInt("RATE_LIMIT_BURST", 40)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		AppPort: getEnv("APP_PORT", "8080"),
		DSN: GetDSN(
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			getEnv("DB_HOST", "localhost"),
			getEnv("DB_PORT", "3306"),
			os.Getenv("DB_DATABASE"),
		),
		MigrationsPath: getEnv("MIGRATIONS_PATH", "file://internal/database/migrations"),
		RedisAddr:      fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "localhost"), getEnv("REDIS_PORT", "6379")),
		RedisPassword:  os.Getenv("REDIS_PASSWORD"),
		RedisDB:        redisDB,
		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTTTL:         jwtTTL,
		TasksCacheTTL:  cacheTTL,
		RateLimitRPS:   rps,
		RateLimitBurst: burst,
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

// GetDSN собирает DSN для MySQL. Все даты храним и читаем в UTC.
func GetDSN(user, password, host, port, dbname string) string {
	c := mysql.NewConfig()
	c.User = user
	c.Passwd = password
	c.Net = "tcp"
	c.Addr = host + ":" + port
	c.DBName = dbname
	c.ParseTime = true
	c.Collation = "utf8mb4_unicode_ci"
	c.Loc = time.UTC
	c.Params = map[string]string{"time_zone": "'+00:00'"}

	return c.FormatDSN()
}

func getEnv(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}

	return n, nil
}

func getEnvDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}

	return d, nil
}

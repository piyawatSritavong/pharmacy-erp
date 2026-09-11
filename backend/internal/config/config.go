package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv                    string
	HTTPPort                  string
	DatabaseURL               string
	JWTSecret                 string
	FrontendURL               string
	CookieName                string
	CookieSecure              bool
	CookieDomain              string
	DefaultVATPct             float64
	SupabaseURL               string
	SupabaseServiceRoleKey    string
	ProductImageBucket        string
	AllowDestructiveSeed      bool
	AllowOperationalDataReset bool
	AllowMasterDataReset      bool
	SeedPOSPassword           string
}

func Load() Config {
	return Config{
		AppEnv: getenv("APP_ENV", "development"),
		// Cloud Run injects PORT and requires the container to listen on it.
		// HTTP_PORT stays for the compose stack, which predates that.
		HTTPPort:                  getenv("PORT", getenv("HTTP_PORT", "8080")),
		DatabaseURL:               getenv("DATABASE_URL", "postgres://pharmacy:pharmacy@localhost:5432/pharmacy_erp?sslmode=disable"),
		JWTSecret:                 getenv("JWT_SECRET", "pharmacy-erp-dev-secret"),
		FrontendURL:               getenv("FRONTEND_URL", "http://localhost:3000"),
		CookieName:                getenv("COOKIE_NAME", "pharmacy_erp_auth"),
		CookieSecure:              getenvBool("COOKIE_SECURE", false),
		CookieDomain:              os.Getenv("COOKIE_DOMAIN"),
		DefaultVATPct:             getenvFloat("DEFAULT_VAT_PCT", 7),
		SupabaseURL:               getenv("SUPABASE_URL", ""),
		SupabaseServiceRoleKey:    getenv("SUPABASE_SERVICE_ROLE_KEY", ""),
		ProductImageBucket:        getenv("PRODUCT_IMAGE_BUCKET", "product-images"),
		AllowDestructiveSeed:      getenvBool("ALLOW_DESTRUCTIVE_SEED", false),
		AllowOperationalDataReset: getenvBool("ALLOW_OPERATIONAL_DATA_RESET", false),
		AllowMasterDataReset:      getenvBool("ALLOW_MASTER_DATA_RESET", false),
		SeedPOSPassword:           getenv("SEED_POS_PASSWORD", "DevPassword123!"),
	}
}

func getenv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

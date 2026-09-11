package config

import (
	"testing"
	"time"
)

func validConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ADMIN_OIDC_AUDIENCE", "baobab-control-plane")
	t.Setenv("ADMIN_OIDC_ISSUER", "http://127.0.0.1:5556")
	t.Setenv("WORKLOAD_OIDC_AUDIENCE", "baobab-control-plane")
	t.Setenv("WORKLOAD_OIDC_ISSUER", "http://127.0.0.1:5557")
}

func TestLoadDefaultsPlatformContextTTL(t *testing.T) {
	validConfigEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.PlatformContextTTL != 15*time.Minute {
		t.Fatalf("expected a 15-minute default PlatformContextTTL, got %s", cfg.PlatformContextTTL)
	}
}

func TestLoadAppliesCustomPlatformContextTTL(t *testing.T) {
	validConfigEnv(t)
	t.Setenv("PLATFORM_CONTEXT_TTL", "5m")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.PlatformContextTTL != 5*time.Minute {
		t.Fatalf("expected the configured 5-minute PlatformContextTTL, got %s", cfg.PlatformContextTTL)
	}
}

func TestLoadRejectsInvalidPlatformContextTTL(t *testing.T) {
	validConfigEnv(t)
	for _, value := range []string{"not-a-duration", "0m", "-5m"} {
		t.Setenv("PLATFORM_CONTEXT_TTL", value)
		if _, err := Load(); err == nil {
			t.Fatalf("expected PLATFORM_CONTEXT_TTL=%q to be rejected", value)
		}
	}
}

func TestLoadRequiresSecureOIDCIssuer(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ADMIN_OIDC_AUDIENCE", "baobab-control-plane")
	t.Setenv("ADMIN_OIDC_ISSUER", "http://identity.example.com")
	t.Setenv("WORKLOAD_OIDC_AUDIENCE", "baobab-control-plane")
	t.Setenv("WORKLOAD_OIDC_ISSUER", "https://workload-identity.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("insecure remote issuer was accepted")
	}
	t.Setenv("ADMIN_OIDC_ISSUER", "http://127.0.0.1:5556")
	if _, err := Load(); err != nil {
		t.Fatalf("local development issuer rejected: %v", err)
	}
	t.Setenv("WORKLOAD_OIDC_ISSUER", "http://workload-identity.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("insecure workload issuer was accepted")
	}
}

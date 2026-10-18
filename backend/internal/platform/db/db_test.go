package db

import (
	"testing"
	"time"
)

func TestPoolConfigSessionSettings(t *testing.T) {
	pc, err := poolConfig(Config{URL: "postgres://app@db:5432/app?sslmode=disable", MaxConns: 7, StatementTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if pc.MaxConns != 7 {
		t.Errorf("MaxConns = %d, want 7", pc.MaxConns)
	}
	want := map[string]string{
		"statement_timeout":                   "5000",
		"lock_timeout":                        "3000",
		"idle_in_transaction_session_timeout": "30000",
		"TimeZone":                            "UTC",
	}
	for k, v := range want {
		if got := pc.ConnConfig.RuntimeParams[k]; got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestPoolConfigRejectsBadURL(t *testing.T) {
	if _, err := poolConfig(Config{URL: "postgres://%zz"}); err == nil {
		t.Fatal("bad URL accepted")
	}
}

package main

import (
	"strings"
	"testing"
)

func TestTestAuthConfigDefaultsDisabled(t *testing.T) {
	var config Config
	if config.TestAuth.Enabled {
		t.Fatal("test authentication must default to disabled")
	}
	if err := validateConfig(config); err != nil {
		t.Fatalf("validateConfig(default) returned error: %v", err)
	}
}

func TestCheckedInConfigLoads(t *testing.T) {
	_, err := loadConfig("cca.scfgs")
	if err != nil {
		t.Fatalf("loadConfig(cca.scfgs) returned error: %v", err)
	}
}

func TestTestAuthRemoteRequiresStrongAccessKey(t *testing.T) {
	var config Config
	config.TestAuth.Enabled = true
	config.TestAuth.AllowRemote = true
	config.TestAuth.AccessKey = "too-short"
	if err := validateConfig(config); err == nil || !strings.Contains(err.Error(), "at least 16") {
		t.Fatalf("validateConfig() error = %v, want minimum access-key error", err)
	}

	config.TestAuth.AccessKey = "development-key-1234"
	if err := validateConfig(config); err != nil {
		t.Fatalf("validateConfig(valid remote mode) returned error: %v", err)
	}
}

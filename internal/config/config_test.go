package config

import (
	"strings"
	"testing"
)

func TestFromEnvAndValidate(t *testing.T) {
	t.Setenv("SULU_URL", "http://sulu.local/")
	t.Setenv("SULU_TOKEN", "tok")
	t.Setenv("SULU_PROJECT_ID", "7")
	t.Setenv("SULU_LAUNCH_NAME", "nightly")
	cfg := FromEnv()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://sulu.local" {
		t.Errorf("trailing slash not stripped: %q", cfg.URL)
	}
	if cfg.ProjectID != 7 || cfg.LaunchName != "nightly" || cfg.Token != "tok" {
		t.Errorf("env not read: %+v", cfg)
	}
}

func TestValidateListsAllMissing(t *testing.T) {
	t.Setenv("SULU_URL", "")
	t.Setenv("SULU_TOKEN", "")
	t.Setenv("SULU_PROJECT_ID", "")
	cfg := FromEnv()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"SULU_URL", "SULU_TOKEN", "SULU_PROJECT_ID"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

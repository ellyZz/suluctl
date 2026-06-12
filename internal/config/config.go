package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config is everything a command needs to talk to Sulu.
// Values come from env first; flags overlay them (flag defaults ARE the env values).
type Config struct {
	URL         string
	Token       string
	ProjectID   int64
	LaunchName  string
	Environment string
	Tags        []string
	EnvVars     map[string]string
	Insecure    bool
}

// FromEnv reads SULU_URL, SULU_TOKEN, SULU_PROJECT_ID, SULU_LAUNCH_NAME.
// A malformed SULU_PROJECT_ID stays 0 and is reported by Validate.
func FromEnv() Config {
	cfg := Config{
		URL:        strings.TrimRight(os.Getenv("SULU_URL"), "/"),
		Token:      os.Getenv("SULU_TOKEN"),
		LaunchName: os.Getenv("SULU_LAUNCH_NAME"),
	}
	if v := os.Getenv("SULU_PROJECT_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.ProjectID = id
		}
	}
	return cfg
}

// Validate returns one error listing every missing required value.
func (c *Config) Validate() error {
	c.URL = strings.TrimRight(c.URL, "/")
	var missing []string
	if c.URL == "" {
		missing = append(missing, "Sulu URL (--url or SULU_URL)")
	}
	if c.Token == "" {
		missing = append(missing, "API token (--token or SULU_TOKEN; create one in Profile → API keys)")
	}
	if c.ProjectID <= 0 {
		missing = append(missing, "project id (--project or SULU_PROJECT_ID)")
	}
	if len(missing) == 0 {
		return nil
	}
	return errors.New("missing required configuration:\n  - " + strings.Join(missing, "\n  - "))
}

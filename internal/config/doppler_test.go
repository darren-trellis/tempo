package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestApplyDopplerEnvNoop(t *testing.T) {
	if err := ApplyDopplerEnv(nil); err != nil {
		t.Fatal(err)
	}
}

func TestApplyDopplerEnvSetsMissing(t *testing.T) {
	t.Setenv("TEMPO_TEST_DOPPLER_KEY", "")
	orig := lookupDopplerSecret
	t.Cleanup(func() { lookupDopplerSecret = orig })
	lookupDopplerSecret = func(project, configName, name string) (string, error) {
		if project != "api" || configName != "stg_backend_ts" || name != "TEMPORAL_TS_API_KEY" {
			t.Fatalf("unexpected lookup %s/%s %s", project, configName, name)
		}
		return "from-doppler", nil
	}

	if err := ApplyDopplerEnv([]DopplerEnv{{
		Env:     "TEMPO_TEST_DOPPLER_KEY",
		Project: "api",
		Config:  "stg_backend_ts",
		Name:    "TEMPORAL_TS_API_KEY",
	}}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("TEMPO_TEST_DOPPLER_KEY"); got != "from-doppler" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyDopplerEnvSkipsSetVars(t *testing.T) {
	t.Setenv("TEMPO_TEST_DOPPLER_KEY", "already")
	orig := lookupDopplerSecret
	t.Cleanup(func() { lookupDopplerSecret = orig })
	lookupDopplerSecret = func(project, configName, name string) (string, error) {
		t.Fatal("should not call Doppler when the env var is already set")
		return "", nil
	}

	if err := ApplyDopplerEnv([]DopplerEnv{{
		Env:     "TEMPO_TEST_DOPPLER_KEY",
		Project: "api",
		Config:  "stg",
		Name:    "TEMPORAL_TS_API_KEY",
	}}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("TEMPO_TEST_DOPPLER_KEY"); got != "already" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyDopplerEnvReportsLookupError(t *testing.T) {
	t.Setenv("TEMPO_TEST_DOPPLER_KEY", "")
	orig := lookupDopplerSecret
	t.Cleanup(func() { lookupDopplerSecret = orig })
	lookupDopplerSecret = func(project, configName, name string) (string, error) {
		return "", fmt.Errorf("unauthorized")
	}

	err := ApplyDopplerEnv([]DopplerEnv{{
		Env:     "TEMPO_TEST_DOPPLER_KEY",
		Project: "api",
		Config:  "prd",
		Name:    "TEMPORAL_TS_API_KEY",
	}})
	if err == nil || !strings.Contains(err.Error(), "TEMPO_TEST_DOPPLER_KEY") || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("got %v", err)
	}
}

func TestParseRoundTripDoppler(t *testing.T) {
	cfg, err := parseConfig([]byte(`
theme: tokyonight-moon
active_profile: staging
doppler:
  - env: TEMPORAL_TS_API_KEY_STG
    project: api
    config: stg_backend_ts
    name: TEMPORAL_TS_API_KEY
profiles:
  staging:
    address: us-west-2.aws.api.temporal.io:7233
    namespace: staging-ts.cvhrv
    api_key: ${TEMPORAL_TS_API_KEY_STG}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Doppler) != 1 || cfg.Doppler[0].Env != "TEMPORAL_TS_API_KEY_STG" || cfg.Doppler[0].Config != "stg_backend_ts" {
		t.Fatalf("doppler=%+v", cfg.Doppler)
	}
	out, err := cfg.marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "stg_backend_ts") || !strings.Contains(string(out), "TEMPORAL_TS_API_KEY_STG") {
		t.Fatalf("marshal dropped doppler:\n%s", out)
	}
}

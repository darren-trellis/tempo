package config

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// DopplerEnv copies one Doppler secret into an environment variable before
// profile fields are expanded.
type DopplerEnv struct {
	Env     string `yaml:"env"`
	Project string `yaml:"project"`
	Config  string `yaml:"config"`
	Name    string `yaml:"name,omitempty"`
}

func (d DopplerEnv) secretName() string {
	if strings.TrimSpace(d.Name) != "" {
		return d.Name
	}
	return d.Env
}

func (d DopplerEnv) validate() error {
	if strings.TrimSpace(d.Env) == "" {
		return fmt.Errorf("doppler entry is missing env")
	}
	if strings.TrimSpace(d.Project) == "" || strings.TrimSpace(d.Config) == "" {
		return fmt.Errorf("doppler entry for %s needs project and config", d.Env)
	}
	return nil
}

type dopplerLookupFunc func(project, configName, name string) (string, error)

var lookupDopplerSecret dopplerLookupFunc = dopplerCLILookup

func dopplerCLILookup(project, configName, name string) (string, error) {
	cmd := exec.Command("doppler", "secrets", "get", name, "--plain", "--project", project, "--config", configName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
				return "", fmt.Errorf("doppler CLI is not installed")
			}
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// ApplyDopplerEnv fetches configured Doppler secrets into the process
// environment. Variables that are already set are left alone.
func ApplyDopplerEnv(entries []DopplerEnv) error {
	if len(entries) == 0 {
		return nil
	}

	type result struct {
		env string
		val string
		err error
	}

	var (
		wg      sync.WaitGroup
		results = make(chan result, len(entries))
	)
	for _, entry := range entries {
		if err := entry.validate(); err != nil {
			return err
		}
		if os.Getenv(entry.Env) != "" {
			continue
		}
		entry := entry
		wg.Add(1)
		go func() {
			defer wg.Done()
			val, err := lookupDopplerSecret(entry.Project, entry.Config, entry.secretName())
			if err != nil {
				results <- result{env: entry.Env, err: fmt.Errorf("%s from %s/%s: %w", entry.Env, entry.Project, entry.Config, err)}
				return
			}
			if strings.TrimSpace(val) == "" {
				results <- result{env: entry.Env, err: fmt.Errorf("%s is empty in Doppler %s/%s", entry.Env, entry.Project, entry.Config)}
				return
			}
			results <- result{env: entry.Env, val: val}
		}()
	}
	wg.Wait()
	close(results)

	var errs []string
	for res := range results {
		if res.err != nil {
			errs = append(errs, res.err.Error())
			continue
		}
		if err := os.Setenv(res.env, res.val); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", res.env, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("loading secrets from Doppler:\n  %s", strings.Join(errs, "\n  "))
}
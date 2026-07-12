package gctx

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const configurationFormat = "json(name,is_active,properties.core.account,properties.core.project,properties.billing.quota_project)"

var humanAccountPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)

type configuration struct {
	Name       string `json:"name"`
	IsActive   bool   `json:"is_active"`
	Properties struct {
		Core struct {
			Account string `json:"account"`
			Project string `json:"project"`
		} `json:"core"`
		Billing struct {
			QuotaProject string `json:"quota_project"`
		} `json:"billing"`
	} `json:"properties"`
}

func decodeConfigurations(data []byte) ([]configuration, error) {
	var configurations []configuration
	if err := json.Unmarshal(data, &configurations); err != nil {
		return nil, fmt.Errorf("decode gcloud configurations: %w", err)
	}
	return configurations, nil
}

func findConfiguration(configurations []configuration, name string) (configuration, error) {
	for _, candidate := range configurations {
		if candidate.Name == name {
			return candidate, nil
		}
	}
	return configuration{}, fmt.Errorf("gcloud configuration %q was not found", name)
}

func currentConfiguration(configurations []configuration) (configuration, error) {
	for _, candidate := range configurations {
		if candidate.IsActive {
			return candidate, nil
		}
	}
	return configuration{}, fmt.Errorf("gcloud has no active configuration")
}

func validateConfiguration(candidate configuration) error {
	if candidate.Properties.Core.Account == "" {
		return fmt.Errorf("configuration %q has no account; run: gcloud config set account ACCOUNT --configuration=%s", candidate.Name, candidate.Name)
	}
	if !humanAccountPattern.MatchString(candidate.Properties.Core.Account) || strings.HasSuffix(strings.ToLower(candidate.Properties.Core.Account), ".gserviceaccount.com") {
		return fmt.Errorf("configuration %q uses unsupported principal %q; gctx v1 supports human user accounts only", candidate.Name, candidate.Properties.Core.Account)
	}
	if candidate.Properties.Core.Project == "" {
		return fmt.Errorf("configuration %q has no project; run: gcloud config set project PROJECT --configuration=%s", candidate.Name, candidate.Name)
	}
	if candidate.Properties.Billing.QuotaProject == "" {
		return fmt.Errorf("configuration %q has no explicit quota project; run: gcloud config set billing/quota_project QUOTA_PROJECT --configuration=%s", candidate.Name, candidate.Name)
	}
	return nil
}

func resultFor(candidate configuration) Result {
	return Result{
		Name:         candidate.Name,
		Account:      candidate.Properties.Core.Account,
		Project:      candidate.Properties.Core.Project,
		QuotaProject: candidate.Properties.Billing.QuotaProject,
	}
}

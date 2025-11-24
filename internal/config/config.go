package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type Target struct {
	InstanceType    string  `json:"instanceType"`
	MaxRuntimeHours float64 `json:"maxRuntimeHours"`
}

type Config struct {
	Targets               []Target
	AWSRegion             string
	SNSTopicArn           string
	Schedule              string
	DryRun                bool
	LeaderElectionEnabled bool
	PodName               string
	PodNamespace          string
	LeaseName             string
}

func Load() (*Config, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		return nil, fmt.Errorf("missing required environment variable: CONFIG_PATH")
	}

	// Load Config File
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer configFile.Close()

	byteValue, _ := io.ReadAll(configFile)

	var targets []Target
	if err := json.Unmarshal(byteValue, &targets); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Parse Env Vars
	cfg := &Config{
		Targets:               targets,
		AWSRegion:             os.Getenv("AWS_REGION"),
		SNSTopicArn:           os.Getenv("SNS_TOPIC_ARN"),
		Schedule:              os.Getenv("SCHEDULE"),
		LeaderElectionEnabled: os.Getenv("LEADER_ELECTION_ENABLED") == "true",
		PodName:               os.Getenv("POD_NAME"),
		PodNamespace:          os.Getenv("POD_NAMESPACE"),
		LeaseName:             os.Getenv("LEASE_NAME"),
	}

	if os.Getenv("DRY_RUN") == "true" {
		cfg.DryRun = true
	}

	if cfg.LeaseName == "" {
		cfg.LeaseName = "ec2-checker-leader"
	}

	return cfg, nil
}

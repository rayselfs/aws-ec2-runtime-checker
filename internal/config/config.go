package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/caarlos0/env/v11"
)

type Target struct {
	// Filter by instance type (optional)
	InstanceType string `json:"instanceType,omitempty"`

	// Filter by instance name (Name tag, supports wildcard matching with *)
	// Example: "my-instance-*", "prod-*"
	Name string `json:"name,omitempty"`

	// Filter by tags (key-value pairs, all must match)
	// Example: {"Environment": "dev", "Team": "backend"}
	Tags map[string]string `json:"tags,omitempty"`

	// Maximum runtime in hours before termination
	MaxRuntimeHours float64 `json:"maxRuntimeHours"`
}

type Config struct {
	Targets               []Target `env:"-"` // Loaded from file, not env
	AWSRegion             string   `env:"AWS_REGION,required"`
	SNSTopicArn           string   `env:"SNS_TOPIC_ARN"`
	Schedule              string   `env:"SCHEDULE"`
	DryRun                bool     `env:"DRY_RUN" envDefault:"true"`
	LeaderElectionEnabled bool     `env:"LEADER_ELECTION_ENABLED"`
	PodName               string   `env:"POD_NAME"`
	PodNamespace          string   `env:"POD_NAMESPACE"`
	LeaseName             string   `env:"LEASE_NAME"`
	VpcID                 string   `env:"VPC_ID"`
	ConfigPath            string   `env:"CONFIG_PATH,required"` // Required env var for config file path
}

func Load() (*Config, error) {
	// Parse environment variables into Config struct
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse environment variables: %w", err)
	}

	// Load Targets from config file
	configFile, err := os.Open(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer configFile.Close()

	byteValue, err := io.ReadAll(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var targets []Target
	if err := json.Unmarshal(byteValue, &targets); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	cfg.Targets = targets

	// Set default LeaseName if not provided
	if cfg.LeaseName == "" {
		cfg.LeaseName = "ec2-checker-leader"
	}

	return cfg, nil
}

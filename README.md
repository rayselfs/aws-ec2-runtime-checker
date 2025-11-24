# AWS EC2 Runtime Checker

<p align="center">
    <a href="https://opensource.org/licenses/Apache-2.0"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" /></a>
    <a href="https://artifacthub.io/packages/search?repo=rayselfs-charts"><img src="https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/rayselfs" /></a>
</p>

Monitors AWS EC2 instances and automatically terminates those exceeding configured runtime thresholds.

## Features

- 🔍 **Instance Monitoring**: Tracks EC2 instances by type and runtime duration
- ⏰ **Flexible Scheduling**: Supports both Kubernetes CronJob and Deployment modes
- 🎯 **Per-Type Configuration**: Set different runtime thresholds for each instance type
- 🛡️ **Dry Run Mode**: Test without actually terminating instances
- 🔔 **SNS Notifications**: Sends alerts before taking action

## Prerequisites

- Kubernetes 1.23+
- Helm 3.8.0+
- AWS credentials with appropriate permissions

## Installation

### Using Helm (Recommended)

```bash
helm repo add aws-ec2-runtime-checker https://rayselfs.github.io/aws-ec2-runtime-checker
helm install aws-ec2-runtime-checker aws-ec2-runtime-checker/aws-ec2-runtime-checker
```

For detailed Helm configuration options, IAM policies, and deployment modes, see the [Helm Chart README](charts/aws-ec2-runtime-checker/README.md).

### From Source

```bash
git clone https://github.com/rayselfs/aws-ec2-runtime-checker
cd aws-ec2-runtime-checker
helm install aws-ec2-runtime-checker ./charts/aws-ec2-runtime-checker
```

## Quick Start

### Basic Configuration

Create a `values.yaml` file:

```yaml
aws:
  region: "us-east-1"

dryRun: true # Set to false to enable actual termination

targets:
  - instanceType: "t2.micro"
    maxRuntimeHours: 24
  - instanceType: "t3.micro"
    maxRuntimeHours: 48
```

Install with custom values:

```bash
helm install aws-ec2-runtime-checker aws-ec2-runtime-checker/aws-ec2-runtime-checker -f values.yaml
```

For complete configuration options and production setup, refer to the [Helm Chart README](charts/aws-ec2-runtime-checker/README.md).

### Config File Format

The `config.json` file (mounted via ConfigMap) should contain:

```json
[
  {
    "instanceType": "t2.micro",
    "maxRuntimeHours": 24
  },
  {
    "instanceType": "t3.micro",
    "maxRuntimeHours": 48
  }
]
```

## Usage

### Deployment Mode (Continuous Monitoring)

```yaml
kind: Deployment
replicaCount: 2
schedule: "*/5 * * * *" # Every 5 minutes

leaderElection:
  enabled: true # Required when replicaCount > 1
```

### CronJob Mode (Scheduled Checks)

```yaml
kind: CronJob
schedule: "0 */6 * * *" # Every 6 hours
```

### Local Development

```bash
# Build
go build -o ec2-checker ./cmd/ec2-checker

# Run single check
export CONFIG_PATH=./config.json
export AWS_REGION=us-east-1
export DRY_RUN=true
./ec2-checker

# Run in cron mode
export SCHEDULE="* * * * *"
./ec2-checker cron
```

## Project Structure

```
.
├── cmd/
│   └── ec2-checker/        # Main application entry point
│       ├── main.go
│       └── main_test.go
├── internal/
│   ├── checker/            # EC2 checking logic
│   │   ├── checker.go
│   │   └── checker_test.go
│   ├── config/             # Configuration management
│   │   ├── config.go
│   │   └── config_test.go
│   └── k8s/                # Kubernetes utilities
│       ├── client.go
│       └── election.go
├── charts/
│   └── ec2-checker/        # Helm chart
└── .github/
    └── workflows/          # CI/CD pipelines
```

## How It Works

1. **Discovery**: Lists all running EC2 instances matching configured types
2. **Filtering**: Applies server-side filtering by instance type for efficiency
3. **Runtime Check**: Calculates runtime since launch time
4. **Action**:
   - Logs instances exceeding thresholds
   - Sends SNS notification (if configured)
   - Terminates instances (unless in dry run mode)
5. **Scheduling**: Waits until next scheduled run (in cron mode)

## Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...
```

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

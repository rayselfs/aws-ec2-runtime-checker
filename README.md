# AWS EC2 Long-Running Checker

This tool checks for AWS EC2 instances of specific types that have been running longer than a configured threshold. It sends notifications via AWS SNS and can optionally terminate the instances.

## Prerequisites

- Go 1.18+
- AWS Credentials configured (e.g., `~/.aws/credentials` or environment variables)
- IAM permissions:
  - `ec2:DescribeInstances`
  - `ec2:TerminateInstances` (if using DELETE mode)
  - `sns:Publish`

## Installation

```bash
git clone <repository-url>
cd ec2-checker
go build -o ec2-checker
```

## Usage

The tool is configured via environment variables:

| Variable            | Description                                                                 | Required | Default |
| ------------------- | --------------------------------------------------------------------------- | -------- | ------- |
| `TARGET_EC2_TYPES`  | Comma-separated list of instance types to check (e.g., `t3.micro,m5.large`) | Yes      | -       |
| `MAX_RUNTIME_HOURS` | Maximum allowed runtime in hours (e.g., `24`, `0.5`)                        | Yes      | -       |
| `SNS_TOPIC_ARN`     | ARN of the SNS topic to send notifications to                               | No       | -       |
| `AWS_REGION`        | AWS Region (e.g., `us-east-1`)                                              | Yes      | -       |
| `DELETE_ENABLE`     | Set to `true` to enable instance termination                                | No       | `false` |

### Example: Notify Only

```bash
export AWS_REGION=us-east-1
export TARGET_EC2_TYPES=t3.micro,t2.micro
export MAX_RUNTIME_HOURS=24
export SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:MyTopic
export DELETE_ENABLE=false

./ec2-checker
```

### Example: Notify and Delete

```bash
export AWS_REGION=us-east-1
export TARGET_EC2_TYPES=c5.large
export MAX_RUNTIME_HOURS=48
export SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:MyTopic
export DELETE_ENABLE=true

./ec2-checker
```

## Logic

1.  **List Instances**: Finds all instances in `running` state.
2.  **Filter**: Keeps only instances matching `TARGET_EC2_TYPES`.
3.  **Check Runtime**: Calculates runtime (`Now - LaunchTime`).
4.  **Action**:
    - If runtime > `MAX_RUNTIME_HOURS`, adds to the report.
    - Sends a single SNS notification with the list of long-running instances.
    - If `DELETE_ENABLE` is `true`, terminates the identified instances immediately.

## Docker

You can run the tool using Docker.

### Build

```bash
docker build -t ec2-checker .
```

### Run

```bash
docker run --rm \
  -e AWS_REGION=us-east-1 \
  -e TARGET_EC2_TYPES=t3.micro \
  -e MAX_RUNTIME_HOURS=24 \
  -e SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456789012:MyTopic \
  -e DELETE_ENABLE=false \
  -e AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID \
  -e AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY \
  -e AWS_SESSION_TOKEN=$AWS_SESSION_TOKEN \
  ec2-checker
```

> [!NOTE]
> Ensure you pass AWS credentials to the container, either via environment variables (as shown) or by mounting your `~/.aws` directory.

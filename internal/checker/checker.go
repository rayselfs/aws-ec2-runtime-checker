package checker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rayselfs/aws-ec2-runtime-checker/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type EC2API interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

type SNSAPI interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type Checker struct {
	EC2Client EC2API
	SNSClient SNSAPI
	Config    *config.Config
}

func New(ec2Client EC2API, snsClient SNSAPI, cfg *config.Config) *Checker {
	return &Checker{
		EC2Client: ec2Client,
		SNSClient: snsClient,
		Config:    cfg,
	}
}

func (c *Checker) RunCheck(ctx context.Context) {
	slog.Info("Checking for long-running instances...")

	var targetTypes []string
	for _, t := range c.Config.Targets {
		targetTypes = append(targetTypes, t.InstanceType)
	}

	input := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
			{
				Name:   aws.String("instance-type"),
				Values: targetTypes,
			},
		},
	}

	paginator := ec2.NewDescribeInstancesPaginator(c.EC2Client, input)
	var longRunningInstances []types.Instance

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			slog.Error("Failed to describe instances", "error", err)
			return
		}

		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				// Filter by Type and Check Runtime
				for _, target := range c.Config.Targets {
					if string(instance.InstanceType) == target.InstanceType {
						launchTime := *instance.LaunchTime
						runtime := time.Since(launchTime)
						if runtime.Hours() > target.MaxRuntimeHours {
							longRunningInstances = append(longRunningInstances, instance)
						}
						break // Found matching target type, move to next instance
					}
				}
			}
		}
	}

	if len(longRunningInstances) == 0 {
		slog.Info("No long-running instances found")
		return
	}

	var messageBuilder strings.Builder
	messageBuilder.WriteString(fmt.Sprintf("Found %d long-running instances:\n", len(longRunningInstances)))

	for _, instance := range longRunningInstances {
		instanceID := *instance.InstanceId
		launchTime := *instance.LaunchTime
		runtime := time.Since(launchTime)

		msg := fmt.Sprintf("- ID: %s, Type: %s, Runtime: %.2f hours\n", instanceID, instance.InstanceType, runtime.Hours())
		messageBuilder.WriteString(msg)
		slog.Info("Found long-running instance", "instance_id", instanceID, "type", instance.InstanceType, "runtime_hours", runtime.Hours())

		if !c.Config.DryRun {
			slog.Info("Terminating instance", "instance_id", instanceID)
			_, err := c.EC2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if err != nil {
				errMsg := fmt.Sprintf("Failed to terminate instance %s: %v\n", instanceID, err)
				messageBuilder.WriteString(errMsg)
				slog.Error("Failed to terminate instance", "instance_id", instanceID, "error", err)
			} else {
				successMsg := fmt.Sprintf("Successfully terminated instance %s\n", instanceID)
				messageBuilder.WriteString(successMsg)
				slog.Info("Successfully terminated instance", "instance_id", instanceID)
			}
		} else {
			slog.Info("DRY RUN: Would terminate instance", "instance_id", instanceID)
		}
	}

	if c.Config.SNSTopicArn != "" {
		slog.Info("Sending SNS notification...")
		_, err := c.SNSClient.Publish(ctx, &sns.PublishInput{
			Message:  aws.String(messageBuilder.String()),
			TopicArn: aws.String(c.Config.SNSTopicArn),
			Subject:  aws.String("Long-Running EC2 Instances Alert"),
		})
		if err != nil {
			slog.Error("Failed to publish to SNS", "error", err)
		}
	} else {
		slog.Info("SNS_TOPIC_ARN not set, skipping notification")
	}
}

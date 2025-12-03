package checker

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
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

	longRunningInstances := c.findLongRunningInstances(ctx)
	if len(longRunningInstances) == 0 {
		slog.Info("No long-running instances found")
		return
	}

	message := c.processInstances(ctx, longRunningInstances)
	c.sendNotification(ctx, message)
}

// buildFilters constructs EC2 API filters based on configuration
func (c *Checker) buildFilters() []types.Filter {
	filters := []types.Filter{
		{
			Name:   aws.String("instance-state-name"),
			Values: []string{"running"},
		},
	}

	// Collect instance types for filtering
	var instanceTypes []string
	for _, t := range c.Config.Targets {
		if t.InstanceType != "" {
			instanceTypes = append(instanceTypes, t.InstanceType)
		}
	}
	if len(instanceTypes) > 0 {
		filters = append(filters, types.Filter{
			Name:   aws.String("instance-type"),
			Values: instanceTypes,
		})
	}

	// Add VPC ID filter if configured
	if c.Config.VpcID != "" {
		filters = append(filters, types.Filter{
			Name:   aws.String("vpc-id"),
			Values: []string{c.Config.VpcID},
		})
	}

	// Collect tag filters
	tagFilters := make(map[string][]string)
	for _, t := range c.Config.Targets {
		for key, value := range t.Tags {
			tagKey := fmt.Sprintf("tag:%s", key)
			if _, exists := tagFilters[tagKey]; !exists {
				tagFilters[tagKey] = []string{}
			}
			tagFilters[tagKey] = append(tagFilters[tagKey], value)
		}
	}
	for tagKey, values := range tagFilters {
		filters = append(filters, types.Filter{
			Name:   aws.String(tagKey),
			Values: values,
		})
	}

	return filters
}

// findLongRunningInstances queries EC2 and filters instances that exceed runtime thresholds
func (c *Checker) findLongRunningInstances(ctx context.Context) []types.Instance {
	filters := c.buildFilters()
	input := &ec2.DescribeInstancesInput{
		Filters: filters,
	}

	paginator := ec2.NewDescribeInstancesPaginator(c.EC2Client, input)
	var longRunningInstances []types.Instance

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			slog.Error("Failed to describe instances", "error", err)
			return nil
		}

		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				if instance := c.checkInstanceRuntime(instance); instance != nil {
					longRunningInstances = append(longRunningInstances, *instance)
				}
			}
		}
	}

	return longRunningInstances
}

// checkInstanceRuntime checks if an instance exceeds any target's runtime threshold
func (c *Checker) checkInstanceRuntime(instance types.Instance) *types.Instance {
	for _, target := range c.Config.Targets {
		if !c.matchesTarget(instance, target) {
			continue
		}

		launchTime := *instance.LaunchTime
		runtime := time.Since(launchTime)
		if runtime.Hours() > target.MaxRuntimeHours {
			return &instance
		}
		break // Found matching target, no need to check others
	}
	return nil
}

// processInstances terminates instances and builds notification message
func (c *Checker) processInstances(ctx context.Context, instances []types.Instance) string {
	var messageBuilder strings.Builder
	messageBuilder.WriteString(fmt.Sprintf("Found %d long-running instances:\n", len(instances)))

	for _, instance := range instances {
		instanceID := *instance.InstanceId
		launchTime := *instance.LaunchTime
		runtime := time.Since(launchTime)

		msg := fmt.Sprintf("- ID: %s, Type: %s, Runtime: %.2f hours\n", instanceID, instance.InstanceType, runtime.Hours())
		messageBuilder.WriteString(msg)
		slog.Info("Found long-running instance", "instance_id", instanceID, "type", instance.InstanceType, "runtime_hours", runtime.Hours())

		if !c.Config.DryRun {
			c.terminateInstance(ctx, instanceID, &messageBuilder)
		} else {
			slog.Info("DRY RUN: Would terminate instance", "instance_id", instanceID)
		}
	}

	return messageBuilder.String()
}

// terminateInstance terminates a single instance and updates the message builder
func (c *Checker) terminateInstance(ctx context.Context, instanceID string, messageBuilder *strings.Builder) {
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
}

// sendNotification sends SNS notification if configured
func (c *Checker) sendNotification(ctx context.Context, message string) {
	if c.Config.SNSTopicArn == "" {
		slog.Info("SNS_TOPIC_ARN not set, skipping notification")
		return
	}

	slog.Info("Sending SNS notification...")
	_, err := c.SNSClient.Publish(ctx, &sns.PublishInput{
		Message:  aws.String(message),
		TopicArn: aws.String(c.Config.SNSTopicArn),
		Subject:  aws.String("Long-Running EC2 Instances Alert"),
	})
	if err != nil {
		slog.Error("Failed to publish to SNS", "error", err)
	}
}

// matchesTarget checks if an instance matches all the filter criteria in a target
// Note: VPC ID filtering is handled at the server-side filter level via Config.VpcID
func (c *Checker) matchesTarget(instance types.Instance, target config.Target) bool {
	// Check instance type (if specified)
	if target.InstanceType != "" && string(instance.InstanceType) != target.InstanceType {
		return false
	}

	// Check Name tag (supports wildcard matching with *)
	if target.Name != "" {
		instanceName := c.getInstanceName(instance)
		matched, err := filepath.Match(target.Name, instanceName)
		if err != nil || !matched {
			return false
		}
	}

	// Check Tags (all specified tags must match)
	if len(target.Tags) > 0 {
		instanceTags := make(map[string]string)
		for _, tag := range instance.Tags {
			if tag.Key != nil && tag.Value != nil {
				instanceTags[*tag.Key] = *tag.Value
			}
		}
		for key, value := range target.Tags {
			if instanceTags[key] != value {
				return false
			}
		}
	}

	return true
}

// getInstanceName extracts the Name tag value from an instance
func (c *Checker) getInstanceName(instance types.Instance) string {
	for _, tag := range instance.Tags {
		if tag.Key != nil && *tag.Key == "Name" && tag.Value != nil {
			return *tag.Value
		}
	}
	return ""
}

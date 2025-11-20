package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

func main() {
	// 1. Parse Environment Variables
	targetTypesEnv := os.Getenv("TARGET_EC2_TYPES")
	maxRuntimeEnv := os.Getenv("MAX_RUNTIME_HOURS")
	deleteEnableEnv := os.Getenv("DELETE_ENABLE")
	snsTopicArn := os.Getenv("SNS_TOPIC_ARN")
	awsRegion := os.Getenv("AWS_REGION")

	if targetTypesEnv == "" || maxRuntimeEnv == "" {
		log.Fatal("Missing required environment variables: TARGET_EC2_TYPES, MAX_RUNTIME_HOURS")
	}

	deleteEnable := false
	if deleteEnableEnv == "true" {
		deleteEnable = true
	}

	targetTypes := strings.Split(targetTypesEnv, ",")
	for i := range targetTypes {
		targetTypes[i] = strings.TrimSpace(targetTypes[i])
	}

	maxRuntimeHours, err := strconv.ParseFloat(maxRuntimeEnv, 64)
	if err != nil {
		log.Fatalf("Invalid MAX_RUNTIME_HOURS: %v", err)
	}

	// 2. Initialize AWS Clients
	ctx := context.TODO()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(awsRegion))
	if err != nil {
		log.Fatalf("Unable to load SDK config, %v", err)
	}

	ec2Client := ec2.NewFromConfig(cfg)
	snsClient := sns.NewFromConfig(cfg)

	// 3. List Running Instances
	log.Println("Checking for long-running instances...")
	input := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running"},
			},
		},
	}

	paginator := ec2.NewDescribeInstancesPaginator(ec2Client, input)
	var longRunningInstances []types.Instance

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("Failed to describe instances: %v", err)
		}

		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				// Filter by Type
				isTargetType := false
				for _, t := range targetTypes {
					if string(instance.InstanceType) == t {
						isTargetType = true
						break
					}
				}
				if !isTargetType {
					continue
				}

				// Check Runtime
				launchTime := *instance.LaunchTime
				runtime := time.Since(launchTime)
				if runtime.Hours() > maxRuntimeHours {
					longRunningInstances = append(longRunningInstances, instance)
				}
			}
		}
	}

	if len(longRunningInstances) == 0 {
		log.Println("No long-running instances found.")
		return
	}

	// 4. Process Long-Running Instances
	var messageBuilder strings.Builder
	messageBuilder.WriteString(fmt.Sprintf("Found %d long-running instances:\n", len(longRunningInstances)))

	for _, instance := range longRunningInstances {
		instanceID := *instance.InstanceId
		launchTime := *instance.LaunchTime
		runtime := time.Since(launchTime)

		msg := fmt.Sprintf("- ID: %s, Type: %s, Runtime: %.2f hours\n", instanceID, instance.InstanceType, runtime.Hours())
		messageBuilder.WriteString(msg)
		log.Print(msg)

		if deleteEnable {
			log.Printf("Terminating instance %s...", instanceID)
			_, err := ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []string{instanceID},
			})
			if err != nil {
				errMsg := fmt.Sprintf("Failed to terminate instance %s: %v\n", instanceID, err)
				messageBuilder.WriteString(errMsg)
				log.Print(errMsg)
			} else {
				successMsg := fmt.Sprintf("Successfully terminated instance %s\n", instanceID)
				messageBuilder.WriteString(successMsg)
				log.Print(successMsg)
			}
		}
	}

	// 5. Send SNS Notification
	if snsTopicArn != "" {
		log.Println("Sending SNS notification...")
		_, err = snsClient.Publish(ctx, &sns.PublishInput{
			Message:  aws.String(messageBuilder.String()),
			TopicArn: aws.String(snsTopicArn),
			Subject:  aws.String("Long-Running EC2 Instances Alert"),
		})
		if err != nil {
			log.Fatalf("Failed to publish to SNS: %v", err)
		}
	} else {
		log.Println("SNS_TOPIC_ARN not set, skipping notification.")
	}

	log.Println("Done.")
}

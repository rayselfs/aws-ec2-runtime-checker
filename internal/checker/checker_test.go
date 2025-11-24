package checker

import (
	"context"
	"testing"
	"time"

	"aws-ec2-runtime-checker/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

// MockEC2Client
type MockEC2Client struct {
	DescribeInstancesFunc  func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	TerminateInstancesFunc func(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
}

func (m *MockEC2Client) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return m.DescribeInstancesFunc(ctx, params, optFns...)
}

func (m *MockEC2Client) TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return m.TerminateInstancesFunc(ctx, params, optFns...)
}

// MockSNSClient
type MockSNSClient struct {
	PublishFunc func(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

func (m *MockSNSClient) Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	return m.PublishFunc(ctx, params, optFns...)
}

func TestRunCheck_LongRunningInstance(t *testing.T) {
	// Setup
	launchTime := time.Now().Add(-25 * time.Hour) // Running for 25 hours
	instanceID := "i-1234567890abcdef0"
	instanceType := "t2.micro"

	mockEC2 := &MockEC2Client{
		DescribeInstancesFunc: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []types.Reservation{
					{
						Instances: []types.Instance{
							{
								InstanceId:   aws.String(instanceID),
								InstanceType: types.InstanceType(instanceType),
								LaunchTime:   &launchTime,
								State: &types.InstanceState{
									Name: types.InstanceStateNameRunning,
								},
							},
						},
					},
				},
			}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
			if len(params.InstanceIds) != 1 || params.InstanceIds[0] != instanceID {
				t.Errorf("Expected to terminate instance %s, got %v", instanceID, params.InstanceIds)
			}
			return &ec2.TerminateInstancesOutput{}, nil
		},
	}

	mockSNS := &MockSNSClient{
		PublishFunc: func(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
			return &sns.PublishOutput{}, nil
		},
	}

	cfg := &config.Config{
		Targets: []config.Target{
			{InstanceType: instanceType, MaxRuntimeHours: 24},
		},
		DryRun:      false,
		SNSTopicArn: "arn:aws:sns:us-east-1:123456789012:mytopic",
	}

	chk := New(mockEC2, mockSNS, cfg)

	// Execute
	chk.RunCheck(context.Background())

	// Verify (Implicitly verified by mock assertions if added, or we can add counters)
}

func TestRunCheck_DryRun(t *testing.T) {
	// Setup
	launchTime := time.Now().Add(-25 * time.Hour)
	instanceID := "i-dryrun"
	instanceType := "t2.micro"

	mockEC2 := &MockEC2Client{
		DescribeInstancesFunc: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return &ec2.DescribeInstancesOutput{
				Reservations: []types.Reservation{
					{
						Instances: []types.Instance{
							{
								InstanceId:   aws.String(instanceID),
								InstanceType: types.InstanceType(instanceType),
								LaunchTime:   &launchTime,
								State: &types.InstanceState{
									Name: types.InstanceStateNameRunning,
								},
							},
						},
					},
				},
			}, nil
		},
		TerminateInstancesFunc: func(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
			t.Error("TerminateInstances should not be called in DryRun mode")
			return nil, nil
		},
	}

	mockSNS := &MockSNSClient{
		PublishFunc: func(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
			return &sns.PublishOutput{}, nil
		},
	}

	cfg := &config.Config{
		Targets: []config.Target{
			{InstanceType: instanceType, MaxRuntimeHours: 24},
		},
		DryRun:      true,
		SNSTopicArn: "arn:aws:sns:us-east-1:123456789012:mytopic",
	}

	chk := New(mockEC2, mockSNS, cfg)

	// Execute
	chk.RunCheck(context.Background())
}

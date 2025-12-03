package checker

import (
	"context"
	"testing"
	"time"

	"github.com/rayselfs/aws-ec2-runtime-checker/internal/config"

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

func TestBuildFilters(t *testing.T) {
	tests := []struct {
		name           string
		config         *config.Config
		expectedFilter string
		expectedValues []string
	}{
		{
			name: "with instance types",
			config: &config.Config{
				Targets: []config.Target{
					{InstanceType: "t2.micro", MaxRuntimeHours: 24},
					{InstanceType: "t3.small", MaxRuntimeHours: 48},
				},
			},
			expectedFilter: "instance-type",
			expectedValues: []string{"t2.micro", "t3.small"},
		},
		{
			name: "with VPC ID",
			config: &config.Config{
				Targets: []config.Target{
					{InstanceType: "t2.micro", MaxRuntimeHours: 24},
				},
				VpcID: "vpc-12345678",
			},
			expectedFilter: "vpc-id",
			expectedValues: []string{"vpc-12345678"},
		},
		{
			name: "with tags",
			config: &config.Config{
				Targets: []config.Target{
					{
						InstanceType:    "t2.micro",
						Tags:            map[string]string{"Environment": "dev", "Team": "backend"},
						MaxRuntimeHours: 24,
					},
				},
			},
			expectedFilter: "tag:Environment",
			expectedValues: []string{"dev"},
		},
		{
			name: "with multiple filters",
			config: &config.Config{
				Targets: []config.Target{
					{
						InstanceType:    "t2.micro",
						Tags:            map[string]string{"Environment": "prod"},
						MaxRuntimeHours: 24,
					},
				},
				VpcID: "vpc-12345678",
			},
			expectedFilter: "instance-type",
			expectedValues: []string{"t2.micro"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chk := &Checker{Config: tt.config}
			filters := chk.buildFilters()

			// Always check for instance-state-name filter
			foundRunningFilter := false
			for _, f := range filters {
				if *f.Name == "instance-state-name" {
					foundRunningFilter = true
					if len(f.Values) != 1 || f.Values[0] != "running" {
						t.Errorf("Expected instance-state-name filter with 'running', got %v", f.Values)
					}
				}
			}
			if !foundRunningFilter {
				t.Error("Expected instance-state-name filter not found")
			}

			// Check for expected filter
			found := false
			for _, f := range filters {
				if *f.Name == tt.expectedFilter {
					found = true
					if len(f.Values) != len(tt.expectedValues) {
						t.Errorf("Expected %d values, got %d", len(tt.expectedValues), len(f.Values))
					}
					for _, expectedValue := range tt.expectedValues {
						valueFound := false
						for _, v := range f.Values {
							if v == expectedValue {
								valueFound = true
								break
							}
						}
						if !valueFound {
							t.Errorf("Expected value %s not found in filter values", expectedValue)
						}
					}
					break
				}
			}
			if !found && tt.expectedFilter != "" {
				t.Errorf("Expected filter %s not found", tt.expectedFilter)
			}
		})
	}
}

func TestCheckInstanceRuntime(t *testing.T) {
	tests := []struct {
		name           string
		instance       types.Instance
		targets        []config.Target
		expectedResult bool
	}{
		{
			name: "instance exceeds runtime threshold",
			instance: types.Instance{
				InstanceId:   aws.String("i-123"),
				InstanceType: types.InstanceType("t2.micro"),
				LaunchTime:   aws.Time(time.Now().Add(-25 * time.Hour)),
			},
			targets: []config.Target{
				{InstanceType: "t2.micro", MaxRuntimeHours: 24},
			},
			expectedResult: true,
		},
		{
			name: "instance within runtime threshold",
			instance: types.Instance{
				InstanceId:   aws.String("i-123"),
				InstanceType: types.InstanceType("t2.micro"),
				LaunchTime:   aws.Time(time.Now().Add(-12 * time.Hour)),
			},
			targets: []config.Target{
				{InstanceType: "t2.micro", MaxRuntimeHours: 24},
			},
			expectedResult: false,
		},
		{
			name: "instance type mismatch",
			instance: types.Instance{
				InstanceId:   aws.String("i-123"),
				InstanceType: types.InstanceType("t3.micro"),
				LaunchTime:   aws.Time(time.Now().Add(-25 * time.Hour)),
			},
			targets: []config.Target{
				{InstanceType: "t2.micro", MaxRuntimeHours: 24},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Targets: tt.targets}
			chk := &Checker{Config: cfg}
			result := chk.checkInstanceRuntime(tt.instance)

			if tt.expectedResult && result == nil {
				t.Error("Expected instance to be flagged as long-running, but got nil")
			}
			if !tt.expectedResult && result != nil {
				t.Error("Expected instance not to be flagged, but got result")
			}
		})
	}
}

func TestMatchesTarget(t *testing.T) {
	tests := []struct {
		name     string
		instance types.Instance
		target   config.Target
		expected bool
	}{
		{
			name: "matches instance type",
			instance: types.Instance{
				InstanceType: types.InstanceType("t2.micro"),
			},
			target: config.Target{
				InstanceType:    "t2.micro",
				MaxRuntimeHours: 24,
			},
			expected: true,
		},
		{
			name: "does not match instance type",
			instance: types.Instance{
				InstanceType: types.InstanceType("t3.micro"),
			},
			target: config.Target{
				InstanceType:    "t2.micro",
				MaxRuntimeHours: 24,
			},
			expected: false,
		},
		{
			name: "matches name tag with wildcard",
			instance: types.Instance{
				InstanceType: types.InstanceType("t2.micro"),
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("dev-instance-01")},
				},
			},
			target: config.Target{
				Name:            "dev-*",
				MaxRuntimeHours: 24,
			},
			expected: true,
		},
		{
			name: "does not match name tag",
			instance: types.Instance{
				InstanceType: types.InstanceType("t2.micro"),
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("prod-instance-01")},
				},
			},
			target: config.Target{
				Name:            "dev-*",
				MaxRuntimeHours: 24,
			},
			expected: false,
		},
		{
			name: "matches tags",
			instance: types.Instance{
				InstanceType: types.InstanceType("t2.micro"),
				Tags: []types.Tag{
					{Key: aws.String("Environment"), Value: aws.String("dev")},
					{Key: aws.String("Team"), Value: aws.String("backend")},
				},
			},
			target: config.Target{
				Tags: map[string]string{
					"Environment": "dev",
					"Team":        "backend",
				},
				MaxRuntimeHours: 24,
			},
			expected: true,
		},
		{
			name: "does not match tags",
			instance: types.Instance{
				InstanceType: types.InstanceType("t2.micro"),
				Tags: []types.Tag{
					{Key: aws.String("Environment"), Value: aws.String("prod")},
				},
			},
			target: config.Target{
				Tags: map[string]string{
					"Environment": "dev",
				},
				MaxRuntimeHours: 24,
			},
			expected: false,
		},
		{
			name: "matches all criteria",
			instance: types.Instance{
				InstanceType: types.InstanceType("t2.micro"),
				Tags: []types.Tag{
					{Key: aws.String("Name"), Value: aws.String("dev-instance-01")},
					{Key: aws.String("Environment"), Value: aws.String("dev")},
				},
			},
			target: config.Target{
				InstanceType: "t2.micro",
				Name:         "dev-*",
				Tags: map[string]string{
					"Environment": "dev",
				},
				MaxRuntimeHours: 24,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chk := &Checker{}
			result := chk.matchesTarget(tt.instance, tt.target)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSendNotification(t *testing.T) {
	tests := []struct {
		name          string
		config        *config.Config
		message       string
		expectPublish bool
	}{
		{
			name: "sends notification when SNS topic is configured",
			config: &config.Config{
				SNSTopicArn: "arn:aws:sns:us-east-1:123456789012:mytopic",
			},
			message:       "Test message",
			expectPublish: true,
		},
		{
			name: "skips notification when SNS topic is not configured",
			config: &config.Config{
				SNSTopicArn: "",
			},
			message:       "Test message",
			expectPublish: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publishCalled := false
			mockSNS := &MockSNSClient{
				PublishFunc: func(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
					publishCalled = true
					if *params.TopicArn != tt.config.SNSTopicArn {
						t.Errorf("Expected topic ARN %s, got %s", tt.config.SNSTopicArn, *params.TopicArn)
					}
					if *params.Message != tt.message {
						t.Errorf("Expected message %s, got %s", tt.message, *params.Message)
					}
					return &sns.PublishOutput{}, nil
				},
			}

			chk := &Checker{
				SNSClient: mockSNS,
				Config:    tt.config,
			}

			chk.sendNotification(context.Background(), tt.message)

			if tt.expectPublish && !publishCalled {
				t.Error("Expected Publish to be called, but it wasn't")
			}
			if !tt.expectPublish && publishCalled {
				t.Error("Expected Publish not to be called, but it was")
			}
		})
	}
}

func TestRunCheck_WithVpcID(t *testing.T) {
	launchTime := time.Now().Add(-25 * time.Hour)
	instanceID := "i-vpc-test"
	instanceType := "t2.micro"
	vpcID := "vpc-12345678"

	mockEC2 := &MockEC2Client{
		DescribeInstancesFunc: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			// Verify VPC filter is applied
			vpcFilterFound := false
			for _, f := range params.Filters {
				if *f.Name == "vpc-id" {
					vpcFilterFound = true
					if len(f.Values) != 1 || f.Values[0] != vpcID {
						t.Errorf("Expected VPC ID filter with %s, got %v", vpcID, f.Values)
					}
				}
			}
			if !vpcFilterFound {
				t.Error("Expected VPC ID filter not found")
			}

			return &ec2.DescribeInstancesOutput{
				Reservations: []types.Reservation{
					{
						Instances: []types.Instance{
							{
								InstanceId:   aws.String(instanceID),
								InstanceType: types.InstanceType(instanceType),
								LaunchTime:   &launchTime,
								VpcId:        aws.String(vpcID),
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
		VpcID:       vpcID,
		DryRun:      false,
		SNSTopicArn: "arn:aws:sns:us-east-1:123456789012:mytopic",
	}

	chk := New(mockEC2, mockSNS, cfg)
	chk.RunCheck(context.Background())
}

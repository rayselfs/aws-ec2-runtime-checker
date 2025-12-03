package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rayselfs/aws-ec2-runtime-checker/internal/checker"
	"github.com/rayselfs/aws-ec2-runtime-checker/internal/config"
	"github.com/rayselfs/aws-ec2-runtime-checker/internal/k8s"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/go-co-op/gocron/v2"

	"k8s.io/client-go/tools/leaderelection"
)

func main() {
	initLogger()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	chk, err := initChecker(ctx, cfg)
	if err != nil {
		slog.Error("Failed to initialize checker", "error", err)
		os.Exit(1)
	}

	if isCronMode() {
		if err := runCronMode(ctx, cfg, chk); err != nil {
			slog.Error("Cron mode failed", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Starting single run...")
		chk.RunCheck(ctx)
	}
}

func initLogger() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
}

func initChecker(ctx context.Context, cfg *config.Config) (*checker.Checker, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	ec2Client := ec2.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)

	return checker.New(ec2Client, snsClient, cfg), nil
}

func isCronMode() bool {
	args := os.Args[1:]
	return len(args) > 0 && args[0] == "cron"
}

func runCronMode(ctx context.Context, cfg *config.Config, chk *checker.Checker) error {
	slog.Info("Starting in cron mode...")

	if cfg.Schedule == "" {
		return fmt.Errorf("SCHEDULE environment variable is required in cron mode")
	}
	slog.Info("Using cron schedule", "schedule", cfg.Schedule)

	if cfg.LeaderElectionEnabled {
		return runWithLeaderElection(ctx, cfg, chk)
	}
	return runSimpleScheduler(ctx, cfg, chk)
}

func runWithLeaderElection(ctx context.Context, cfg *config.Config, chk *checker.Checker) error {
	if cfg.PodName == "" || cfg.PodNamespace == "" {
		return fmt.Errorf("POD_NAME and POD_NAMESPACE are required for leader election")
	}

	k8sClient, err := k8s.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	electionCfg := k8s.ElectionConfig{
		LeaseName:     cfg.LeaseName,
		Namespace:     cfg.PodNamespace,
		PodName:       cfg.PodName,
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
	}

	callbacks := leaderelection.LeaderCallbacks{
		OnStartedLeading: func(ctx context.Context) {
			slog.Info("Became leader, starting check loop...")
			if err := runSimpleScheduler(ctx, cfg, chk); err != nil {
				slog.Error("Scheduler failed", "error", err)
			}
		},
		OnStoppedLeading: func() {
			slog.Info("Lost leadership, exiting...")
			os.Exit(0)
		},
		OnNewLeader: func(identity string) {
			if identity == cfg.PodName {
				slog.Info("I am the leader!")
			} else {
				slog.Info("New leader elected", "leader", identity)
			}
		},
	}

	k8s.RunLeaderElection(ctx, k8sClient, electionCfg, callbacks)
	return nil
}

func runSimpleScheduler(ctx context.Context, cfg *config.Config, chk *checker.Checker) error {
	s, err := gocron.NewScheduler()
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	_, err = s.NewJob(
		gocron.CronJob(cfg.Schedule, false),
		gocron.NewTask(func() {
			chk.RunCheck(ctx)
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	s.Start()
	slog.Info("Scheduler started")

	// Run once immediately
	chk.RunCheck(ctx)

	// Block until context is done
	<-ctx.Done()

	if err := s.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown scheduler: %w", err)
	}
	return nil
}

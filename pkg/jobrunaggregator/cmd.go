package jobrunaggregator

import (
	"context"
	"flag"
	"fmt"

	"cloud.google.com/go/storage"
	"github.com/openshift/ci-tools/pkg/jobrunaggregator/jobrunaggregatoranalyzer"
	"github.com/openshift/ci-tools/pkg/jobrunaggregator/jobrunaggregatorcachebuilder"
	"google.golang.org/api/option"
)

const (
	openshiftCIBucket string = "origin-ci-test"
)

type Runnable interface {
	Run(ctx context.Context) error
}

// JobRunAggregatorFlags is used to configure the command and produce the runtime structure
type JobRunAggregatorFlags struct {
	Subcommand string

	// common args
	JobName    string
	WorkingDir string

	// analyzer args
	PayloadTag string
}

func NewJobRunAggregatorFlags() *JobRunAggregatorFlags {
	return &JobRunAggregatorFlags{
		WorkingDir: "job-aggregator-working-dir",
	}
}

// TODO having a bind and a parse via pflags would make this more kube-like
func (f *JobRunAggregatorFlags) ParseFlags(args []string) error {
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)

	fs.StringVar(&f.Subcommand, "subcommand", f.Subcommand, "the subcommand. maybe add cobra. one of [cache, analyze]")
	fs.StringVar(&f.JobName, "job", f.JobName, "The name of the job to inspect, like periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade")
	fs.StringVar(&f.WorkingDir, "working-dir", f.WorkingDir, "The directory to store caches, output, and the like.")

	fs.StringVar(&f.PayloadTag, "payload-tag", f.PayloadTag, "The payload tag to aggregate, like 4.9.0-0.ci-2021-07-19-185802")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return nil
}

// Validate checks to see if the user-input is likely to produce functional runtime options
func (f *JobRunAggregatorFlags) Validate() error {
	if len(f.Subcommand) == 0 {
		return fmt.Errorf("missing --subcommand: one of [cache, analyze]")
	}
	if len(f.JobName) == 0 {
		return fmt.Errorf("missing --job: like periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade")
	}
	if len(f.WorkingDir) == 0 {
		return fmt.Errorf("missing --working-dir: like job-aggregator-working-dir")
	}

	switch f.Subcommand {
	case "cache":
	case "analyze":
		if len(f.PayloadTag) == 0 {
			return fmt.Errorf("missing --payload-tag: like 4.9.0-0.ci-2021-07-19-185802")
		}

	default:
		return fmt.Errorf("--subcommand must be one of [cache, analyze]")
	}

	return nil
}

// ToOptions goes from the user input to the runtime values need to run the command.
// Expect to see unit tests on the options, but not on the flags which are simply value mappings.
func (f *JobRunAggregatorFlags) ToOptions(ctx context.Context) (Runnable, error) {

	switch f.Subcommand {
	case "cache":
		// Create a new GCS Client
		gcsClient, err := storage.NewClient(ctx, option.WithoutAuthentication())
		if err != nil {
			return nil, err
		}

		return &jobrunaggregatorcachebuilder.JobRunAggregatorCacheBuilderOptions{
			JobName:    f.JobName,
			GCSClient:  gcsClient,
			WorkingDir: f.WorkingDir,
		}, nil

	case "analyze":
		return &jobrunaggregatoranalyzer.JobRunAggregatorAnalyzerOptions{
			JobName:    f.JobName,
			WorkingDir: f.WorkingDir,
			PayloadTag: f.PayloadTag,
		}, nil

	default:
		return nil, fmt.Errorf("--subcommand must be one of [cache, analyze]")
	}

	return nil, fmt.Errorf("--subcommand must be one of [cache, analyze]")
}

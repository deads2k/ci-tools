package jobrunaggregator

import (
	"flag"
	"fmt"
)

// JobRunAggregatorFlags is used to configure the command and produce the runtime structure
type JobRunAggregatorFlags struct {
	JobName string
}

func NewJobRunAggregatorFlags() *JobRunAggregatorFlags {
	return &JobRunAggregatorFlags{}
}

// TODO having a bind and a parse via pflags would make this more kube-like
func (f *JobRunAggregatorFlags) ParseFlags(args []string) error {
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)

	fs.StringVar(&f.JobName, "job", f.JobName, "The name of the job to inspect.")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return nil
}

// Validate checks to see if the user-input is likely to produce functional runtime options
func (f *JobRunAggregatorFlags) Validate() error {
	return nil
}

// ToOptions goes from the user input to the runtime values need to run the command.
// Expect to see unit tests on the options, but not on the flags which are simply value mappings.
func (f *JobRunAggregatorFlags) ToOptions() (*JobRunAggregatorOptions, error) {
	return &JobRunAggregatorOptions{}, nil
}

// JobRunAggregatorOptions is the runtime struct that is produced from the parsed flags
type JobRunAggregatorOptions struct {
	JobName string
}

func (o *JobRunAggregatorOptions) Run() error {
	fmt.Printf("Aggregating job runs of type %v.", o.JobName)
	return nil
}

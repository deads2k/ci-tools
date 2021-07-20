package jobrunaggregatoranalyzer

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/openshift/ci-tools/pkg/jobrunaggregator/jobrunaggregatorapi"
)

// JobRunAggregatorAnalyzerOptions
// 1. reads a local cache of prowjob.json and junit files for a particular job.
// 2. finds jobruns for the the specified payload tag
// 3. reads all junit for the each jobrun
// 4. constructs a synthentic junit that includes every test and assigns pass/fail to each test
type JobRunAggregatorAnalyzerOptions struct {
	JobName    string
	WorkingDir string
	PayloadTag string
}

func (o *JobRunAggregatorAnalyzerOptions) Run(ctx context.Context) error {
	fmt.Printf("Analyzing job runs of type %q.\n", o.JobName)

	jobDir := filepath.Join(o.WorkingDir, "logs", o.JobName)
	fmt.Printf("Reading job runs from  %q.\n", jobDir)
	jobRunDirs, err := ioutil.ReadDir(jobDir)
	if err != nil {
		return err
	}
	jobRuns := []jobrunaggregatorapi.JobRun{}
	for _, jobRunDir := range jobRunDirs {
		jobRunID := filepath.Base(jobRunDir.Name())
		jobRun, err := jobrunaggregatorapi.NewFilesystemJobRun(o.WorkingDir, o.JobName, jobRunID)
		if err != nil {
			return err
		}
		jobRuns = append(jobRuns, jobRun)
	}

	for _, jobRun := range jobRuns {
		prowJob, err := jobRun.GetProwJob(ctx)
		if err != nil {
			return err
		}
		if _, ok := prowJob.Labels["release.openshift.io/analysis"]; ok {
			// this structure is one we can work against
			nameTargetDir := filepath.Join(o.WorkingDir, "by-name", o.JobName, prowJob.Labels["release.openshift.io/analysis"], prowJob.Name)
			nameTargetFile := filepath.Join(nameTargetDir, "prowjob.yaml")
			prowJobBytes, err := jobrunaggregatorapi.SerializeProwJob(prowJob)
			if err != nil {
				return err
			}

			if err := os.MkdirAll(nameTargetDir, 0755); err != nil {
				return err
			}
			if err := ioutil.WriteFile(nameTargetFile, prowJobBytes, 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

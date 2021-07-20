package jobrunaggregatorcachebuilder

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/openshift/ci-tools/pkg/jobrunaggregator/jobrunaggregatorapi"

	"cloud.google.com/go/storage"
)

const (
	openshiftCIBucket string = "origin-ci-test"
)

// JobRunAggregatorCacheBuilderOptions reads prowjob.json and junit files for the specified job and caches them
// to the local disk for use by other processes.
type JobRunAggregatorCacheBuilderOptions struct {
	JobName    string
	GCSClient  *storage.Client
	WorkingDir string
}

func (o *JobRunAggregatorCacheBuilderOptions) Run(ctx context.Context) error {
	fmt.Printf("Caching job runs of type %v.\n", o.JobName)
	jobRuns, err := o.ReadProwJobs(ctx)
	if err != nil {
		return err
	}

	for _, jobRun := range jobRuns {
		// to match GCS bucket
		if err := jobRun.WriteCache(ctx, o.WorkingDir); err != nil {
			return err
		}

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

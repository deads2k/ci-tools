package jobrunaggregatorcachebuilder

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift/ci-tools/pkg/jobrunaggregator/jobrunaggregatorapi"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	utiltrace "k8s.io/utils/trace"
)

func (o *JobRunAggregatorCacheBuilderOptions) ReadProwJobs(ctx context.Context) ([]jobrunaggregatorapi.JobRun, error) {
	fmt.Printf("Reading prowjobs for job %v.\n", o.JobName)

	jobRuns, err := o.getProwJobPathsForJob(ctx)
	if err != nil {
		return nil, err
	}

	// Iterate through the ProwJob paths, retrieve the objects and decode them into a struct for further processing
	for _, jobRun := range jobRuns {
		// call made to fill the content
		if _, err := jobRun.GetAllContent(ctx); err != nil {
			return nil, err
		}
		logrus.Infof("retrieved all content for %q", jobRun.GetJobRunID())
	}

	return jobRuns, nil
}

func (o *JobRunAggregatorCacheBuilderOptions) traceFields() []utiltrace.Field {
	return []utiltrace.Field{
		{Key: "jobName", Value: o.JobName},
	}
}

func (o *JobRunAggregatorCacheBuilderOptions) getProwJobPathsForJob(ctx context.Context) ([]jobrunaggregatorapi.JobRun, error) {
	trace := utiltrace.New("GetProwJobs", o.traceFields()...)
	defer trace.LogIfLong(500 * time.Millisecond)

	prowJobRuns := []jobrunaggregatorapi.JobRun{}
	runIDToJobRun := map[string]jobrunaggregatorapi.JobRun{}

	bkt := o.GCSClient.Bucket(openshiftCIBucket)

	query := &storage.Query{
		// This ends up being the equivalent of:
		// https://gcsweb-ci.apps.ci.l2s4.p1.openshiftapps.com/gcs/origin-ci-test/logs/periodic-ci-openshift-release-master-nightly-4.9-upgrade-from-stable-4.8-e2e-metal-ipi-upgrade
		Prefix: "logs/" + o.JobName,

		// TODO this field is apparently missing from this level of go/storage
		// Omit owner and ACL fields for performance
		//Projection: storage.ProjectionNoACL,
	}

	// Only retrieve the name and creation time for performance
	if err := query.SetAttrSelection([]string{"Name", "Created"}); err != nil {
		return nil, err
	}
	// TODO need to discover this based on our current cache.
	query.StartOffset = "logs/periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade/1416792177459073024"
	trace.Step("Query configured.")

	now := time.Now()

	// Returns an iterator which iterates over the bucket query results.
	// Unfortunately, this will list *all* files with the query prefix.
	it := bkt.Objects(ctx, query)
	trace.Step("Iterator retrieved.")

	// Find the query results we're the most interested in. In this case, we're
	// interested in files called prowjob.json that were created less than 24
	// hours ago.
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return prowJobRuns, err
		}

		// TODO refine time
		if now.Sub(attrs.Created) > (24 * time.Hour) {
			continue
		}

		switch {
		case strings.HasSuffix(attrs.Name, "prowjob.json"):
			logrus.Infof("Found %s", attrs.Name)
			jobRunId := filepath.Base(filepath.Dir(attrs.Name))
			newJobRun := runIDToJobRun[jobRunId]
			if newJobRun == nil {
				newJobRun = jobrunaggregatorapi.NewGCSJobRun(bkt, o.JobName, jobRunId)
				runIDToJobRun[jobRunId] = newJobRun
				prowJobRuns = append(prowJobRuns, newJobRun)
			}
			newJobRun.SetGCSProwJobPath(attrs.Name)

		case strings.HasSuffix(attrs.Name, ".xml") && strings.Contains(attrs.Name, "/junit"):
			logrus.Infof("Found %s", attrs.Name)
			nameParts := strings.Split(attrs.Name, "/")
			if len(nameParts) < 4 {
				continue
			}
			jobRunId := nameParts[2]
			newJobRun := runIDToJobRun[jobRunId]
			if newJobRun == nil {
				newJobRun = jobrunaggregatorapi.NewGCSJobRun(bkt, o.JobName, jobRunId)
				runIDToJobRun[jobRunId] = newJobRun
				prowJobRuns = append(prowJobRuns, newJobRun)
			}
			newJobRun.AddGCSJunitPaths(attrs.Name)

		default:
			//fmt.Printf("checking %q\n", attrs.Name)
		}

	}
	trace.Step("List filtered.")

	// eliminate items without prowjob.json
	ret := []jobrunaggregatorapi.JobRun{}
	for i := range prowJobRuns {
		jobRun := prowJobRuns[i]
		if len(jobRun.GetGCSJunitPaths()) == 0 {
			fmt.Printf("Removing %q because it doesn't have a prowjob.json\n", jobRun.GetJobRunID())
			continue
		}
		ret = append(ret, jobRun)
	}

	return ret, nil
}

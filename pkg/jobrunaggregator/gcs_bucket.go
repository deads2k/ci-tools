package jobrunaggregator

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	goyaml "gopkg.in/yaml.v2"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	utiltrace "k8s.io/utils/trace"
)

type JobRun struct {
	// retrieval mechanisms
	bkt *storage.BucketHandle

	JobRunID       string
	GCSProwJobPath string
	GCSJunitPaths  []string

	pathToContent map[string][]byte
}

func (o *JobRunAggregatorOptions) ReadProwJobs(ctx context.Context) ([]*JobRun, error) {
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
		logrus.Infof("retrieved all content for %s", jobRun.GCSProwJobPath)
	}

	return jobRuns, nil
}

func (o *JobRunAggregatorOptions) traceFields() []utiltrace.Field {
	return []utiltrace.Field{
		{Key: "jobName", Value: o.JobName},
	}
}

func (o *JobRunAggregatorOptions) getProwJobPathsForJob(ctx context.Context) ([]*JobRun, error) {
	trace := utiltrace.New("GetProwJobs", o.traceFields()...)
	defer trace.LogIfLong(500 * time.Millisecond)

	prowJobRuns := []*JobRun{}
	runIDToJobRun := map[string]*JobRun{}

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
				newJobRun = NewJobRunPathsFromGCS(bkt, jobRunId)
				runIDToJobRun[jobRunId] = newJobRun
				prowJobRuns = append(prowJobRuns, newJobRun)
			}
			newJobRun.GCSProwJobPath = attrs.Name

		case strings.HasSuffix(attrs.Name, ".xml") && strings.Contains(attrs.Name, "/junit"):
			logrus.Infof("Found %s", attrs.Name)
			nameParts := strings.Split(attrs.Name, "/")
			if len(nameParts) < 4 {
				continue
			}
			jobRunId := nameParts[2]
			newJobRun := runIDToJobRun[jobRunId]
			if newJobRun == nil {
				newJobRun = NewJobRunPathsFromGCS(bkt, jobRunId)
				runIDToJobRun[jobRunId] = newJobRun
				prowJobRuns = append(prowJobRuns, newJobRun)
			}
			newJobRun.GCSJunitPaths = append(newJobRun.GCSJunitPaths, attrs.Name)

		default:
			//fmt.Printf("checking %q\n", attrs.Name)
		}

	}
	trace.Step("List filtered.")

	// eliminate items without prowjob.json
	ret := []*JobRun{}
	for i := range prowJobRuns {
		jobRun := prowJobRuns[i]
		if len(jobRun.GCSProwJobPath) == 0 {
			fmt.Printf("Removing %q because it doesn't have a prowjob.json\n", jobRun.JobRunID)
			continue
		}
		ret = append(ret, jobRun)
	}

	return ret, nil
}

func NewJobRunPathsFromGCS(bkt *storage.BucketHandle, jobRunId string) *JobRun {
	return &JobRun{
		bkt:      bkt,
		JobRunID: jobRunId,
	}
}

func (j *JobRun) WriteCache(ctx context.Context, parentDir string) error {
	if err := j.writeCache(ctx, parentDir); err != nil {
		// attempt to remove the dir so we don't leave half the content serialized out
		_ = os.Remove(parentDir)
		return err
	}

	return nil
}

func (j *JobRun) writeCache(ctx context.Context, parentDir string) error {
	prowJob, err := j.GetProwJob(ctx)
	if err != nil {
		return err
	}
	prowJobBytes, err := serializeProwJob(prowJob)
	if err != nil {
		return err
	}

	contentMap, err := j.GetAllContent(ctx)
	if err != nil {
		return err
	}
	for path, content := range contentMap {
		currentTargetFilename := filepath.Join(parentDir, path)
		immediateParentDir := filepath.Dir(currentTargetFilename)
		if err := os.MkdirAll(immediateParentDir, 0755); err != nil {
			return err
		}
		if err := ioutil.WriteFile(currentTargetFilename, content, 0644); err != nil {
			return err
		}

		if strings.HasSuffix(currentTargetFilename, "prowjob.json") {
			if err := ioutil.WriteFile(filepath.Join(immediateParentDir, "prowjob.yaml"), prowJobBytes, 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

func (j *JobRun) GetProwJob(ctx context.Context) (*prowjobv1.ProwJob, error) {
	if len(j.GCSProwJobPath) == 0 {
		return nil, fmt.Errorf("missing prowjob path")
	}
	prowBytes, err := j.GetContent(ctx, j.GCSProwJobPath)
	if err != nil {
		return nil, err
	}
	return parseProwJob(prowBytes)
}

func (j *JobRun) GetContent(ctx context.Context, path string) ([]byte, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("missing path")
	}
	if content, ok := j.pathToContent[path]; ok {
		return content, nil
	}

	// Get an Object handle for the path
	obj := j.bkt.Object(path)

	// Get an io.Reader for the object.
	gcsReader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer gcsReader.Close()

	return ioutil.ReadAll(gcsReader)
}

func (j *JobRun) GetAllContent(ctx context.Context) (map[string][]byte, error) {
	if len(j.pathToContent) > 0 {
		return j.pathToContent, nil
	}

	errs := []error{}
	ret := map[string][]byte{}

	allPaths := []string{j.GCSProwJobPath}
	allPaths = append(allPaths, j.GCSJunitPaths...)
	for _, path := range allPaths {
		var err error
		ret[path], err = j.GetContent(ctx, path)
		if err != nil {
			errs = append(errs, err)
		}
	}
	err := utilerrors.NewAggregate(errs)
	if err != nil {
		return nil, err
	}

	j.pathToContent = ret

	return ret, nil
}

func parseProwJob(prowJobBytes []byte) (*prowjobv1.ProwJob, error) {
	prowJob := &prowjobv1.ProwJob{}
	err := yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(prowJobBytes), 4096).Decode(&prowJob)
	if err != nil {
		return nil, err
	}
	prowJob.ManagedFields = nil

	return prowJob, nil
}

func serializeProwJob(prowJob *prowjobv1.ProwJob) ([]byte, error) {
	buf := &bytes.Buffer{}
	prowJobWriter := goyaml.NewEncoder(buf)
	if err := prowJobWriter.Encode(prowJob); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

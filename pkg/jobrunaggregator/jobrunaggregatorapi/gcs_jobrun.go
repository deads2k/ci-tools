package jobrunaggregatorapi

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

type gcsJobRun struct {
	// retrieval mechanisms
	bkt              *storage.BucketHandle
	workingDirectory string

	jobName        string
	jobRunID       string
	gcsProwJobPath string
	gcsJunitPaths  []string

	pathToContent map[string][]byte
}

func NewGCSJobRun(bkt *storage.BucketHandle, jobName, jobRunID string) JobRun {
	return &gcsJobRun{
		bkt:      bkt,
		jobName:  jobName,
		jobRunID: jobRunID,
	}
}

func (j *gcsJobRun) GetJobName() string {
	return j.jobName
}
func (j *gcsJobRun) GetJobRunID() string {
	return j.jobRunID
}
func (j *gcsJobRun) GetGCSProwJobPath() string {
	return j.gcsProwJobPath
}
func (j *gcsJobRun) GetGCSJunitPaths() []string {
	return j.gcsJunitPaths
}
func (j *gcsJobRun) SetGCSProwJobPath(gcsProwJobPath string) {
	j.gcsProwJobPath = gcsProwJobPath
}
func (j *gcsJobRun) AddGCSJunitPaths(junitPaths ...string) {
	j.gcsJunitPaths = append(j.gcsJunitPaths, junitPaths...)
}

func (j *gcsJobRun) WriteCache(ctx context.Context, parentDir string) error {
	if err := j.writeCache(ctx, parentDir); err != nil {
		// attempt to remove the dir so we don't leave half the content serialized out
		_ = os.Remove(parentDir)
		return err
	}

	return nil
}

func (j *gcsJobRun) writeCache(ctx context.Context, parentDir string) error {
	prowJob, err := j.GetProwJob(ctx)
	if err != nil {
		return err
	}
	prowJobBytes, err := SerializeProwJob(prowJob)
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

func (j *gcsJobRun) GetProwJob(ctx context.Context) (*prowjobv1.ProwJob, error) {
	if len(j.gcsProwJobPath) == 0 {
		return nil, fmt.Errorf("missing prowjob path")
	}
	prowBytes, err := j.GetContent(ctx, j.gcsProwJobPath)
	if err != nil {
		return nil, err
	}
	return ParseProwJob(prowBytes)
}

func (j *gcsJobRun) GetContent(ctx context.Context, path string) ([]byte, error) {
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

func (j *gcsJobRun) GetAllContent(ctx context.Context) (map[string][]byte, error) {
	if len(j.pathToContent) > 0 {
		return j.pathToContent, nil
	}

	errs := []error{}
	ret := map[string][]byte{}

	allPaths := []string{j.gcsProwJobPath}
	allPaths = append(allPaths, j.gcsJunitPaths...)
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

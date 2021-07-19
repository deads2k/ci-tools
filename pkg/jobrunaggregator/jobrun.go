package jobrunaggregator

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

type JobRun struct {
	// retrieval mechanisms
	bkt *storage.BucketHandle

	JobRunID       string
	GCSProwJobPath string
	GCSJunitPaths  []string

	pathToContent map[string][]byte
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

package jobrunaggregatorapi

import (
	"bytes"
	"context"

	goyaml "gopkg.in/yaml.v2"

	"k8s.io/apimachinery/pkg/util/yaml"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

// JobRun is a way to interact with JobRuns and gather their junit results.
// The backing store can vary by impl, but GCS buckets and
type JobRun interface {
	GetJobName() string
	GetJobRunID() string
	GetGCSProwJobPath() string
	GetGCSJunitPaths() []string
	SetGCSProwJobPath(gcsProwJobPath string)
	AddGCSJunitPaths(junitPaths ...string)

	GetProwJob(ctx context.Context) (*prowjobv1.ProwJob, error)
	GetContent(ctx context.Context, path string) ([]byte, error)
	GetAllContent(ctx context.Context) (map[string][]byte, error)

	WriteCache(ctx context.Context, parentDir string) error
}

func ParseProwJob(prowJobBytes []byte) (*prowjobv1.ProwJob, error) {
	prowJob := &prowjobv1.ProwJob{}
	err := yaml.NewYAMLOrJSONDecoder(bytes.NewBuffer(prowJobBytes), 4096).Decode(&prowJob)
	if err != nil {
		return nil, err
	}
	prowJob.ManagedFields = nil

	return prowJob, nil
}

func SerializeProwJob(prowJob *prowjobv1.ProwJob) ([]byte, error) {
	buf := &bytes.Buffer{}
	prowJobWriter := goyaml.NewEncoder(buf)
	if err := prowJobWriter.Encode(prowJob); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
// The purpose of this tool is to read a peribolos configuration
// file, get the admins/members of a given organization and
// update the users of a specific group in an Openshift cluster.
package main

import (
	"os"

	"github.com/sirupsen/logrus"

	"github.com/openshift/ci-tools/pkg/jobrunaggregator"
)

func main() {
	f := jobrunaggregator.NewJobRunAggregatorFlags()
	if err := f.ParseFlags(os.Args); err != nil {
		logrus.WithError(err).Fatal("Failed to parse flags")
	}
	if err := f.Validate(); err != nil {
		logrus.WithError(err).Fatal("Flags are invalid")
	}
	o, err := f.ToOptions()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to build runtime options")
	}

	if err := o.Run(); err != nil {
		logrus.WithError(err).Fatal("Command failed")
	}
}

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/common"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/downloader/jenkinsdownloader"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/jpi"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/utils"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

const (
	maxWorkers = 10
)

var (
	workingDir = flag.String("working-dir", filepath.Join(os.Getenv("HOME"), ".jpr"), "plugins working directory, default to the HOME/.jenkins directory")
	outputDir  = flag.String("output-dir", filepath.Join(os.Getenv("JENKINS_HOME"), "plugins"), "output directory, default to the JENKINS_HOME/plugins directory")
	inputFile  = flag.String("input", "plugins.json.lock", "input file. You can use the output of jpresolver")
)

func readInput() (*api.PluginsRegistry, error) {
	plugins := &api.PluginsRegistry{}
	if err := utils.UnmarshalFile(*inputFile, plugins); err != nil {
		return nil, errors.Trace(err)
	}
	return plugins, nil
}

// This function will copy the downloaded plugins (from the working directory
// used to as fs cache) to the output directory. It will rename them to match
// the jenkins requirements (no version in the filename).
func copyPlugins(plugins *api.PluginsRegistry) error {
	var errs error
	for _, p := range plugins.Plugins {
		// Use anonymous function to free descriptors in each loop iteration.
		err := func() error {
			src := jpi.GetPluginPath(p, *workingDir)
			r, err := os.Open(src)
			if err != nil {
				return errors.Trace(err)
			}
			defer r.Close()

			dst := filepath.Join(*outputDir, fmt.Sprintf("%s.jpi", p.Name))
			w, err := os.Create(dst)
			if err != nil {
				return errors.Trace(err)
			}
			defer w.Close()

			_, err = io.Copy(w, r)
			return errors.Trace(err)
		}()
		errs = multierror.Append(errs, err)
	}
	return errs
}

func run() error {
	if err := validateFlags(); err != nil {
		flag.Usage()
		return errors.Trace(err)
	}

	plugins, err := readInput()
	if err != nil {
		return errors.Trace(err)
	}

	downloader := jenkinsdownloader.NewDownloader()
	if err := jpi.RunWorkersPoll(plugins, downloader, *workingDir, maxWorkers); err != nil {
		return errors.Trace(err)
	}

	return copyPlugins(plugins)
}

func validateFlags() error {
	if *inputFile == "" {
		return errors.Errorf("undefined input file")
	}
	if ok, err := utils.FileExists(*outputDir); err != nil {
		return errors.Trace(err)
	} else if !ok {
		return errors.Errorf("the output directory does not exist")
	}
	if err := common.EnsureStorePathExists(*workingDir, jpi.GetStorePath); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("%+v", err)
	}

	log.Printf("done!")
}

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/common"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/downloader/jenkinsdownloader"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/graph"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/jpi"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/meta"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/war"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/utils"
	"github.com/golang/protobuf/jsonpb"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

const (
	maxWorkers = 10
)

var (
	inputFile  = flag.String("input", "plugins.json", "input file (.json, .jsonnet. .yaml or .yml)")
	warFile    = flag.String("war", "jenkins.war", "jenkins war file")
	optional   = flag.Bool("optional", false, "add optional dependencies to the output. It will allow plugins to run with all the expected features.")
	showGraph  = flag.Bool("show-graph", false, "show whole dependencies graph in JSON")
	workingDir = flag.String("working-dir", filepath.Join(os.Getenv("HOME"), ".jpr"), "plugins working dir")
)

func lockPlugins(pr *api.PluginsRegistry, input string) (*api.PluginsRegistry, error) {
	downloader := jenkinsdownloader.NewDownloader()
	g, err := graph.FetchGraph(pr, downloader, *workingDir, maxWorkers)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if *showGraph {
		m := jsonpb.Marshaler{Indent: "  "}
		if err := m.Marshal(os.Stdout, g); err != nil {
			return nil, errors.Trace(err)
		}
	}

	lock, err := graph.LockPlugins(g, *optional)
	if err != nil {
		return nil, errors.Trace(err)
	}

	incs, err := graph.FindIncompatibilities(pr, lock, g)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(incs) > 0 {
		log.Printf(" There were found some incompatibilities within %s:\n", filepath.Base(input))
		incs.Print()
	}

	return lock, nil
}

func readInput() (*api.Project, error) {
	project := &api.Project{}
	if err := utils.UnmarshalFile(*inputFile, project); err != nil {
		return nil, errors.Trace(err)
	}
	return project, nil
}

func writeOutput(pr *api.PluginsRegistry) error {
	outputFile := fmt.Sprintf("%s-lock.%s", strings.TrimSuffix(*inputFile, filepath.Ext(*inputFile)), "json")
	sort.Sort(api.ByName(pr.Plugins))
	return utils.MarshalJSON(outputFile, pr)
}

func run() error {
	if err := validateFlags(); err != nil {
		flag.Usage()
		return errors.Trace(err)
	}

	project, err := readInput()
	if err != nil {
		return errors.Trace(err)
	}
	plugins := project.GetPluginsRegistry()

	jk, err := war.Read(*warFile, *workingDir)
	if err != nil {
		return errors.Trace(err)
	}
	jkpr := war.NewPluginsRegistry(jk)

	// Lock the input plugins
	lock, err := lockPlugins(plugins, *inputFile)
	if err != nil {
		return errors.Trace(err)
	}

	// Lock the jenkins plugins
	jkLock, err := lockPlugins(jkpr, *warFile)
	if err != nil {
		return errors.Trace(err)
	}

	// Add missing jenkins bundled plugins to the locked plugins
	bics, err := war.AggregateBundledPlugins(plugins, jkLock, lock)
	if err != nil {
		return errors.Trace(err)
	}
	if len(bics) > 0 {
		log.Printf(" There were found some incompatibilities within %s:\n", filepath.Base(*warFile))
		bics.Print()
	}

	return writeOutput(lock)
}

func validateFlags() error {
	if ok, err := utils.FileExists(*inputFile); err != nil {
		return errors.Trace(err)
	} else if !ok {
		return errors.Errorf("%s does not exist", *inputFile)
	}

	// Ensure working paths exist
	var errs error
	for _, fn := range []func(string) string{
		graph.GetStorePath,
		jpi.GetStorePath,
		meta.GetStorePath,
		war.GetStorePath,
	} {
		if err := common.EnsureStorePathExists(*workingDir, fn); err != nil {
			errs = multierror.Append(errs, errors.Trace(err))
			continue
		}
	}
	return errs
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("%+v", err)
	}
}

package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/downloader/jenkinsdownloader"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/graph"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/jpi"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/meta"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/utils"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/juju/errors"
)

const (
	maxWorkers = 10
)

var (
	workingDir = flag.String("working-dir", filepath.Join(os.Getenv("HOME"), ".jenkins"), "plugins working dir")
	inputFile  = flag.String("input", "plugins.json", "input file (.json, .jsonnet. .yaml or .yml)")
	outputFile = flag.String("output", "", "output file. If not provided, it will default to <input>.lock")
	optional   = flag.Bool("optional", false, "add optional dependencies to the output. It will allow plugins to run with all the expected features.")
	showGraph  = flag.Bool("show-graph", false, "show whole dependencies graph in JSON")
)

func resolve(pr *api.PluginsRegistry) (*api.PluginsRegistry, error) {
	downloader := jenkinsdownloader.NewDownloader()
	g, err := graph.FetchGraph(pr, downloader, *inputFile, *workingDir, maxWorkers)
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
		log.Printf(" There were found some incompatibilities:\n")
		incs.PrintIncompatibilities()
	}

	return lock, nil
}

func readInput() (*api.Project, error) {
	var unmarshal func(string, proto.Message) error
	switch filepath.Ext(*inputFile) {
	case ".json":
		unmarshal = utils.UnmarshalJSON
	case ".jsonnet":
		unmarshal = utils.UnmarshalJsonnet
	case ".yaml", ".yml":
		unmarshal = utils.UnmarshalYAML
	}
	if unmarshal == nil {
		return nil, errors.Errorf("unsupported input file type: %s\n", *inputFile)
	}
	project := &api.Project{}
	if err := unmarshal(*inputFile, project); err != nil {
		return nil, errors.Trace(err)
	}
	return project, nil
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

	pr, err := resolve(plugins)
	if err != nil {
		return errors.Trace(err)
	}

	if err := utils.MarshalJSON(*outputFile, pr); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func validateFlags() error {
	if ok, err := utils.FileExists(*inputFile); err != nil {
		return errors.Trace(err)
	} else if !ok {
		return errors.Errorf("%s does not exist", *inputFile)
	}
	if *outputFile == "" {
		*outputFile = *inputFile + ".lock"
	}

	// Ensure working paths exist
	if err := graph.EnsureStorePathExists(*workingDir); err != nil {
		return errors.Trace(err)
	}
	if err := jpi.EnsureStorePathExists(*workingDir); err != nil {
		return errors.Trace(err)
	}
	if err := meta.EnsureStorePathExists(*workingDir); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("%+v", err)
	}
}

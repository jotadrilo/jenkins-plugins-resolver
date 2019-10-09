package graph

import (
	"context"
	"log"
	"sort"
	"time"

	api "github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/crypto"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/downloader/common"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/meta"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/utils"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

const (
	timeoutMin = 5
)

// WriteGraph will write a graph into a file
func WriteGraph(g *api.Graph, graphPath string) error {
	return utils.MarshalJSON(graphPath, g)
}

// ReadGraph reads a graph from the store
func ReadGraph(graphPath string) (*api.Graph, error) {
	g := &api.Graph{}
	return g, utils.UnmarshalJSON(graphPath, g)
}

// NewNode will return a graph node for the given plugin
func NewNode(p *api.Plugin, workingDir string) (*api.Graph_Node, error) {
	node := api.Graph_Node{
		Plugin: p,
	}
	dependencies := []*api.Graph_Node{}
	optionalDependencies := []*api.Graph_Node{}

	metaPath := meta.GetMetaPath(p, workingDir)
	pm, err := meta.ReadMetadata(metaPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, dep := range pm.Dependencies {
		nodeDep, err := NewNode(dep, workingDir)
		if err != nil {
			return nil, errors.Trace(err)
		}
		dependencies = append(dependencies, nodeDep)
	}
	node.Dependencies = dependencies

	for _, dep := range pm.OptionalDependencies {
		nodeDep, err := NewNode(dep, workingDir)
		if err != nil {
			return nil, errors.Trace(err)
		}
		optionalDependencies = append(optionalDependencies, nodeDep)
	}
	node.OptionalDependencies = optionalDependencies

	return &node, nil
}

// FetchGraph computes the graph for a list of plugins or read it from the store
func FetchGraph(plugins *api.PluginsRegistry, d common.Downloader, workingDir string, maxWorkers int) (*api.Graph, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeoutMin*time.Minute)
	defer cancel()

	// NOTE: We need to ensure that the list of plugins are properly
	//       sorted before computing its hash.
	sort.Sort(api.ByName(plugins.Plugins))
	hash, err := crypto.SHA256(plugins)
	if err != nil {
		return nil, errors.Trace(err)
	}

	graphPath := GetGraphPath(hash, workingDir)
	cached, err := utils.FileExists(graphPath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if cached {
		log.Printf("Reading graph from disk: %s\n", graphPath)
		return ReadGraph(graphPath)
	}

	log.Println("Computing graph...")
	if err := fetch(ctx, plugins, d, workingDir, maxWorkers); err != nil {
		return nil, errors.Trace(err)
	}

	// Iterate the provided list of plugins first to initialize the map
	var errs error
	var nodes []*api.Graph_Node
	for _, p := range plugins.Plugins {
		node, err := NewNode(p, workingDir)
		if err != nil {
			errs = multierror.Append(errs, errors.Trace(err))
			continue
		}
		nodes = append(nodes, node)
	}
	if errs != nil {
		return nil, errors.Trace(errs)
	}

	g := api.Graph{
		Nodes: nodes,
	}
	if err := WriteGraph(&g, graphPath); err != nil {
		return nil, errors.Trace(err)
	}
	log.Printf("Recorded graph to disk: %s\n", graphPath)

	return &g, nil
}

func fetch(ctx context.Context, plugins *api.PluginsRegistry, d common.Downloader, workingDir string, maxWorkers int) error {
	var errs error

	// Iterate the provided list of plugins first to fetch the metadata from upstream
	if err := meta.RunWorkersPoll(plugins, d, workingDir, maxWorkers); err != nil {
		return errors.Trace(err)
	}

	// Read metadata to fetch the metadata for them
	for _, p := range plugins.Plugins {
		metaPath := meta.GetMetaPath(p, workingDir)
		pm, err := meta.ReadMetadata(metaPath)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		// Iterate dependencies
		depPluginsRegistry := api.PluginsRegistry{
			Plugins: pm.Dependencies,
		}
		if err := fetch(ctx, &depPluginsRegistry, d, workingDir, maxWorkers); err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		// Iterate optional dependencies
		optDepPluginsRegistry := api.PluginsRegistry{
			Plugins: pm.OptionalDependencies,
		}
		if err := fetch(ctx, &optDepPluginsRegistry, d, workingDir, maxWorkers); err != nil {
			errs = multierror.Append(errs, err)
			continue
		}
	}
	return errs
}

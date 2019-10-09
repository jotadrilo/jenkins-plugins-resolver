package graph

import (
	"sort"

	"github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/utils"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

type pluginsMap map[string]string

// newPluginsRegistry iterates a map of plugins and creates a new plugins registry
func (pm pluginsMap) newPluginsRegistry() *api.PluginsRegistry {
	names := make([]string, 0, len(pm))
	for name := range pm {
		names = append(names, name)
	}
	sort.Strings(names)

	plugins := []*api.Plugin{}
	for _, name := range names {
		p := api.Plugin{
			Name:    name,
			Version: pm[name],
		}
		plugins = append(plugins, &p)
	}
	return &api.PluginsRegistry{
		Plugins: plugins,
	}
}

func updatePluginsMap(pm pluginsMap, p *api.Plugin) error {
	if ok, err := utils.VersionLower(pm[p.Name], p.Version); err != nil {
		return errors.Trace(err)
	} else if !ok {
		return nil
	}
	pm[p.Name] = p.Version
	return nil
}

// This method will resolve all the required dependencies and update the plugins
// map with any newer (and new) found plugin version.
//
// It will iterate over the graph nodes by accessing dependencies recursively.
// If we want to resolve optional dependencies too (-optional flag) we will also
// iterate over the optional dependencies, as we can consider them regular ones.
func resolveNodeDependencies(n *api.Graph_Node, pm pluginsMap, optional bool) error {
	if err := updatePluginsMap(pm, n.Plugin); err != nil {
		return errors.Trace(err)
	}
	for _, nd := range n.Dependencies {
		if err := resolveNodeDependencies(nd, pm, optional); err != nil {
			return errors.Trace(err)
		}
	}
	if optional {
		for _, nd := range n.OptionalDependencies {
			if err := resolveNodeDependencies(nd, pm, optional); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// This method will resolve all the "required optional dependencies" and update the plugins
// map with any newer (and new) found plugin version.
//
// Required optional dependencies are those already added dependencies (regular dependencies).
// Optional dependencies are not optional anymore if another plugin depends directly on it.
//
// Example:
//  - a:1.0 has b:2.0 as optional dep
//  - c:1.0 has b:1.0 as dep
//
// Therefore, Jenkins will complain about b:1.0 being older than required (b is
// installed now and, therefore, required to meet the requirements of a:1.0)
//
// It seems this is not a documented behavior:
// https://wiki.jenkins.io/display/JENKINS/Dependencies+among+plugins
func resolveNodeOptionalDependencies(n *api.Graph_Node, pm pluginsMap, optional bool) error {
	// If we don't want optional dependencies to be included in the output,
	// we will only process those optional dependencies that have been already
	// added to the map (they are real dependencies for another plugin)
	if !optional && pm[n.Plugin.Name] == "" {
		return nil
	}
	if err := updatePluginsMap(pm, n.Plugin); err != nil {
		return errors.Trace(err)
	}
	for _, nd := range n.OptionalDependencies {
		if err := resolveNodeOptionalDependencies(nd, pm, optional); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// LockPlugins generates a fully-qualified registry of plugins from a graph
func LockPlugins(g *api.Graph, optional bool) (*api.PluginsRegistry, error) {
	// Auxiliar mapping to set plugins by name
	pm := make(pluginsMap)

	// We need to resolve nodes dependencies first because they might include
	// plugins that are optional dependencies for others.
	var errs error
	for _, n := range g.Nodes {
		if err := resolveNodeDependencies(n, pm, optional); err != nil {
			errs = multierror.Append(errs, err)
			continue
		}
	}
	for _, n := range g.Nodes {
		if err := resolveNodeOptionalDependencies(n, pm, optional); err != nil {
			errs = multierror.Append(errs, err)
			continue
		}
	}
	if errs != nil {
		return nil, errors.Trace(errs)
	}

	return pm.newPluginsRegistry(), nil
}

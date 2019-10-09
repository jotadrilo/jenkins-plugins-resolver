package graph

import (
	"fmt"
	"strings"

	api "github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/common"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/utils"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

// FindIncompatibilities walks through a graph if there are missmatches between a plugin registry and its
// locked version
func FindIncompatibilities(pr *api.PluginsRegistry, lock *api.PluginsRegistry, g *api.Graph) (common.Incompatibilities, error) {
	var errs error
	incompatibilities := make(common.Incompatibilities)
	for _, ip := range pr.Plugins {
		var p *api.Plugin
		for _, op := range lock.Plugins {
			if ip.Name == op.Name {
				p = op
				break
			}
		}
		if p == nil {
			errs = multierror.Append(errs, errors.Errorf("unable to find %s in the locked file", ip.Identifier()))
			continue
		}
		lower, err := utils.VersionLower(ip.Version, p.Version)
		if err != nil {
			errs = multierror.Append(errs, errors.Trace(err))
			continue
		}

		if lower {
			reqs := []string{}
			for _, n := range g.Nodes {
				reqs = findPluginRequesters(n, p, reqs, []string{})
			}
			incompatibilities[ip.Identifier()] = common.Incompatibility{
				Cause:      fmt.Sprintf("a newer version was locked: %s", p.Identifier()),
				Requesters: reqs,
			}
		}
	}
	return incompatibilities, errs
}

func findPluginRequesters(n *api.Graph_Node, p *api.Plugin, r []string, aux []string) []string {
	if p.Identifier() == n.Plugin.Identifier() {
		aux := append(aux, n.Plugin.Identifier())
		tree := strings.Join(aux, " > ")
		r = append(r, tree)
	} else {
		aux = append(aux, n.Plugin.Identifier())
	}
	for _, nd := range n.Dependencies {
		r = findPluginRequesters(nd, p, r, aux)
	}
	for _, nd := range n.OptionalDependencies {
		r = findPluginRequesters(nd, p, r, aux)
	}
	return r
}

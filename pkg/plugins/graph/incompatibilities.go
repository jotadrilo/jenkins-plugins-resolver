package graph

import (
	"log"
	"strings"

	api "github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/juju/errors"
	"github.com/mmikulicic/multierror"
)

// FindIncompatibilities walks through a graph if there are missmatches between two list of plugins requests
func FindIncompatibilities(g *api.Graph, ipr *api.PluginsRequest, opr *api.PluginsRequest) error {
	var errs error
	var found bool
	incompatibilities := make(map[string][]string)
	for _, ip := range ipr.Plugins {
		var p *api.Plugin
		for _, op := range opr.Plugins {
			if ip.Name == op.Name {
				p = op
				break
			}
		}
		if p == nil {
			errs = multierror.Append(errs, errors.Errorf("unable to find %s in the output plugins request", ip.Identifier()))
		}
		if ip.Version != p.Version {
			found = true
			reqs := []string{}
			for _, n := range g.Nodes {
				reqs = findPluginRequesters(n, p, reqs, []string{})
			}
			incompatibilities[ip.Identifier()] = reqs
		}
	}
	if found {
		log.Printf(" There were found some incompatibilities:\n")
		for inc, reqs := range incompatibilities {
			log.Printf("  ├── %s:\n", inc)
			for nr, r := range reqs {
				if nr == len(reqs)-1 {
					log.Printf("  │   └── %s\n", r)
				} else {
					log.Printf("  │   ├── %s\n", r)
				}
			}
		}
		log.Printf("\n You should bump the version in the input and evaluate the required changes.\n")
		errs = multierror.Append(errs, errors.Errorf("found incompatibilities"))
	}
	return errs
}

func findPluginRequesters(n *api.Graph_Node, p *api.Plugin, r []string, aux []string) []string {
	if p.Version == n.Plugin.Version {
		aux := append(aux, n.Plugin.Identifier())
		tree := strings.Join(aux, " > ")
		r = append(r, tree)
	} else {
		aux = append(aux, n.Plugin.Identifier())
	}
	for _, nd := range n.Dependencies {
		// auxDep = append(aux)
		r = findPluginRequesters(nd, p, r, aux)
	}
	return r
}
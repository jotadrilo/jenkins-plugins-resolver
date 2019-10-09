package war

import (
	"fmt"
	"log"

	api "github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/common"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/utils"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

// AggregateBundledPlugins walks through a jenkins bundled plugins locked registry and compare their versions with the inputs locked registry.
// In case of finding a mismatch (jenkins locked plugin is different than the input locked plugin):
//
//  |    Lower    | Present in the project file |      Higher      |          Actions               |
//  |-------------|-----------------------------|------------------|--------------------------------|
//  |      OK     |             YES             |  Incompatibility | Ask to update the project file |
//  |      OK     |             NO              |       OK         |      Override the lock         |
//
func AggregateBundledPlugins(pr *api.PluginsRegistry, jkLock *api.PluginsRegistry, lock *api.PluginsRegistry) (common.Incompatibilities, error) {
	presentInProjectFile := func(p *api.Plugin) bool {
		for _, plugin := range pr.GetPlugins() {
			if plugin.GetName() == p.GetName() {
				return true
			}
		}
		return false
	}

	var errs error
	incompatibilities := make(common.Incompatibilities)
	for _, jklp := range jkLock.GetPlugins() {
		found := false
		for nlp, lp := range lock.GetPlugins() {
			if jklp.GetName() == lp.GetName() {
				found = true
				if jklp.GetVersion() != lp.GetVersion() {
					// If the jenkins locked plugin is lower than the user project locked plugin, it is fine
					lower, err := utils.VersionLower(jklp.GetVersion(), lp.GetVersion())
					if err != nil {
						errs = multierror.Append(errs, errors.Trace(err))
						continue
					}
					if lower {
						continue
					}
					// If it is present in the project file, ask to update it
					if presentInProjectFile(jklp) {
						incompatibilities[fmt.Sprintf("jenkins.war > %s", jklp.Identifier())] = common.Incompatibility{
							Cause:      fmt.Sprintf("a newer version was bundled in the Jenkins war, please update your project file or remove %s from it", lp.Identifier()),
							Requesters: []string{fmt.Sprintf("%s: %s -> %s", lp.GetName(), lp.GetVersion(), jklp.GetVersion())},
						}
						break
					}
					// Otherwise, we should honor the jenkins bundled plugin
					log.Printf("overwriting locked plugin %s with Jenkins bundled plugin %s\n", lp.Identifier(), jklp.Identifier())
					lock.Plugins[nlp] = jklp
				}
			}
		}
		if !found {
			// If the plugin is not present in the user project locked plugins, we should add the jenkins bundled plugin
			log.Printf("adding Jenkins bundled plugin to the locked list: %s\n", jklp.Identifier())
			lock.Plugins = append(lock.Plugins, jklp)
		}
	}

	return incompatibilities, errs
}

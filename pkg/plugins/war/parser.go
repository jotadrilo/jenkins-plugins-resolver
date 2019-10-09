package war

import (
	"regexp"

	"github.com/bitnami-labs/jenkins-plugins-resolver/api"
	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/jpi"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

var (
	versionRegex        = regexp.MustCompile(`Jenkins-Version:\s*([^\r]+)\r?\n`)
	detachedPluginRegex = regexp.MustCompile(`Name:\s*(WEB-INF/detached-plugins/[^\r]+)\r?\n`)
)

// JenkinsManifest represents the Jenkins manifest file
type JenkinsManifest struct {
	Version      string
	PluginsFiles []string
}

// ParseManifest parses a Jenkins manifest file content
func ParseManifest(manifest string) (*JenkinsManifest, error) {
	matches := versionRegex.FindStringSubmatch(manifest)
	if len(matches) == 0 {
		return nil, errors.Errorf("cannot match %q in %q", versionRegex, manifest)
	}
	version := matches[1]

	detachedPlugins, err := getDetachedPlugins(manifest)
	if err != nil {
		return nil, errors.Trace(err)
	}

	jm := &JenkinsManifest{
		PluginsFiles: detachedPlugins,
		Version:      version,
	}

	return jm, nil
}

func getDetachedPlugins(manifest string) ([]string, error) {
	matches := detachedPluginRegex.FindAllStringSubmatch(manifest, -1)
	if len(matches) == 0 {
		return nil, errors.Errorf("cannot match %q in %q", detachedPluginRegex, manifest)
	}

	var detachedPlugins []string
	for _, name := range matches {
		detachedPlugins = append(detachedPlugins, name[1])
	}

	return detachedPlugins, nil
}

// Parse takes a jenkins manifest and returns a Jenkins struct
func (jm JenkinsManifest) Parse(warfile string) (*api.Jenkins, error) {
	manifests, err := ExtractDetachedPluginsManifests(warfile, jm.PluginsFiles)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var errs error
	var detachedPlugins []*api.PluginMetadata
	for _, manifest := range manifests {
		pm, err := jpi.ParseManifest(manifest)
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}
		detachedPlugins = append(detachedPlugins, pm)
	}

	jenkins := &api.Jenkins{
		Version: jm.Version,
		Plugins: detachedPlugins,
	}

	return jenkins, nil
}

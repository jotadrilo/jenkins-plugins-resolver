package war

import (
	"io/ioutil"
	"os"

	"github.com/bitnami-labs/jenkins-plugins-resolver/pkg/plugins/jar"
	zipper "github.com/bitnami-labs/jenkins-plugins-resolver/pkg/zip"
	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

const (
	manifestPath = "META-INF/MANIFEST.MF"
)

type manifestResponse struct {
	Error    error
	Manifest string
	Plugin   string
}

// ExtractDetachedPluginsManifests takes a war and returns a reader to a detached plugin manifest content
func ExtractDetachedPluginsManifests(war string, detachedPlugins []string) (map[string]string, error) {
	detachedPluginsBytes, err := zipper.ExtractFiles(war, detachedPlugins)
	if err != nil {
		return nil, errors.Trace(err)
	}

	manifestsChannel := make(chan manifestResponse, len(detachedPlugins))
	for dp, bytes := range detachedPluginsBytes {
		go func(dpName string, dpBytes []byte) {
			// Create temp file for storing the plugin
			file, err := ioutil.TempFile("", "plugin")
			if err != nil {
				manifestsChannel <- manifestResponse{Plugin: dpName, Error: errors.Trace(err)}
				return
			}
			defer os.Remove(file.Name())

			if err := ioutil.WriteFile(file.Name(), dpBytes, 0644); err != nil {
				manifestsChannel <- manifestResponse{Plugin: dpName, Error: errors.Trace(err)}
				return
			}

			manifest, err := jar.ExtractManifest(file.Name())
			if err != nil {
				manifestsChannel <- manifestResponse{Plugin: dpName, Error: errors.Trace(err)}
				return
			}
			manifestsChannel <- manifestResponse{Plugin: dpName, Manifest: manifest}
		}(dp, bytes)
	}

	var errs error
	detachedPluginsManifests := make(map[string]string, len(detachedPlugins))
	for i := 0; i < len(detachedPlugins); i++ {
		mr := <-manifestsChannel
		if mr.Error != nil {
			errs = multierror.Append(errs, errors.Annotatef(mr.Error, "unable to extract manifest from %s", mr.Plugin))
			continue
		}
		detachedPluginsManifests[mr.Plugin] = mr.Manifest
	}

	return detachedPluginsManifests, errs
}

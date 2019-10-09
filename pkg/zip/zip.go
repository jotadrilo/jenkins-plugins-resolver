package zip

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/juju/errors"
	"github.com/mkmik/multierror"
)

// OpenFile finds a file `src` from the provided reader `w` for a zip file and returns a reader to it
func OpenFile(r *os.File, src string) (io.ReadCloser, error) {
	stat, err := r.Stat()
	if err != nil {
		return nil, errors.Trace(err)
	}

	zr, err := zip.NewReader(r, stat.Size())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// we will get out of the loop when EOF
	for _, f := range zr.File {
		if f.Name != src {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		return rc, nil
	}

	return nil, errors.Errorf("unable to find %s in the zip file", src)
}

// ExtractFiles takes a zip and returns a map to the bytes of target files inside it.
func ExtractFiles(zipname string, filenames []string) (map[string][]byte, error) {
	r, err := os.Open(zipname)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer r.Close()

	if mimetype, err := GetFileMimeType(r); err != nil {
		return nil, errors.Trace(err)
	} else if mimetype != "application/zip" {
		return nil, errors.Errorf("%s is not a valid zip file", zipname)
	}

	filesnamesMap := make(map[string][]byte, len(filenames))

	// TODO(jdrios) compare performance vs using "OpenFiles" (not implemented)
	//				ram vs speed when using large zip files
	var errs error
	for _, dp := range filenames {
		err := func() error {
			rc, err := OpenFile(r, dp)
			if err != nil {
				return errors.Trace(err)
			}
			defer rc.Close()

			data, err := ioutil.ReadAll(rc)
			if err != nil {
				return errors.Trace(err)
			}

			filesnamesMap[dp] = data
			return nil
		}()
		errs = multierror.Append(errs, err)
	}

	return filesnamesMap, errs
}

// ExtractFile takes a zip and returns the bytes of target file inside it.
func ExtractFile(zipname string, filename string) ([]byte, error) {
	filenamesMap, err := ExtractFiles(zipname, []string{filename})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return filenamesMap[filename], nil
}

// GetFileMimeType guesses the mimetype of a file from reading its first 512 bytes
func GetFileMimeType(f *os.File) (string, error) {
	text := make([]byte, 512)
	// We need to capture the number of bytes ReadAt returns and slice text to that
	// length. Otherwise, text would contain extra data that is not in the file to
	// DetectContentType.
	nbr, err := f.ReadAt(text, 0)
	if err != nil {
		return "", nil
	}
	return http.DetectContentType(text[:nbr]), nil
}

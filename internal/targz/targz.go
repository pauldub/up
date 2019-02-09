package targz

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/mholt/archiver"
	"github.com/pkg/errors"
	archive "github.com/tj/go-archive"
)

var transform = archive.TransformFunc(func(r io.Reader, i os.FileInfo) (io.Reader, os.FileInfo) {
	name := strings.Replace(i.Name(), "\\", "/", -1)

	i = archive.Info{
		Name:     name,
		Size:     i.Size(),
		Mode:     i.Mode() | 0555,
		Modified: i.ModTime(),
		Dir:      i.IsDir(),
	}.FileInfo()

	return r, i
})

func Build(dir string) (io.ReadCloser, *archive.Stats, error) {
	upignore, err := read(".upignore")
	if err != nil {
		return nil, nil, errors.Wrap(err, "reading .upignore")
	}
	defer upignore.Close()

	r := io.MultiReader(
		strings.NewReader(".*\n"),
		strings.NewReader("\n!node_modules/**\n!.pypath/**\n"),
		upignore,
		strings.NewReader("\n!main\n!server\n!_proxy.js\n!byline.js\n!up.json\n!pom.xml\n!build.gradle\n!project.clj\ngin-bin\nup\n"),
	)

	filter, err := archive.FilterPatterns(r)
	if err != nil {
		return nil, nil, errors.Wrap(err, "parsing ignore patterns")
	}

	buf := new(bytes.Buffer)
	stats := archive.Stats{}
	tar := archiver.NewTar()
	if err := tar.Create(buf); err != nil {
		return nil, nil, errors.Wrap(err, "creating")
	}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == "." {
			return nil
		}

		info = &pathInfo{info, path}
		if filter != nil && filter.Match(info) {
			if info.IsDir() {
				atomic.AddInt64(&stats.DirsFiltered, 1)
				return filepath.SkipDir
			}

			atomic.AddInt64(&stats.FilesFiltered, 1)
			return nil
		}

		if info.IsDir() {
			return nil
		}

		atomic.AddInt64(&stats.FilesAdded, 1)
		atomic.AddInt64(&stats.SizeUncompressed, info.Size())

        /*
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(info.Name())
			if err != nil {
				return errors.Wrap(err, "reading symlink")
			}

            f, err := 
			w, err := tar.Write(archiver.FileInfo{
                FileInfo: info,
                ReadCloser: link,
            })
			if err != nil {
				return errors.Wrap(err, "adding file")
			}

			return nil
		}*/

		f, err := os.Open(path)
		if err != nil {
			return errors.Wrap(err, "opening file")
		}

		var r io.Reader = f
		if transform != nil {
			r, info = transform.Transform(r, info)
		}

		err = tar.Write(archiver.File{
            FileInfo: info,
            ReadCloser: ioutil.NopCloser(r),
        })
		if err != nil {
			return errors.Wrap(err, "adding file")
		}

		if err := f.Close(); err != nil {
			return errors.Wrap(err, "closing file")
		}

		return nil
	})
    if err := tar.Close(); err != nil {
        return nil, nil, errors.Wrap(err, "close")
    }

    gzbuf := new(bytes.Buffer)
    gz := archiver.NewGz()
    if err := gz.Compress(buf, gzbuf); err != nil {
        return nil, nil, errors.Wrap(err, "compress")
    }

	return ioutil.NopCloser(gzbuf), &stats, nil
}

// read file.
func read(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)

	if os.IsNotExist(err) {
		return ioutil.NopCloser(bytes.NewReader(nil)), nil
	}

	if err != nil {
		return nil, err
	}

	return f, nil
}

// pathInfo wraps FileInfo to support a full path
// in place of Name(), which just makes the API
// a little simpler.
type pathInfo struct {
	os.FileInfo
	path string
}

// Name returns the full path.
func (p *pathInfo) Name() string {
	return p.path
}

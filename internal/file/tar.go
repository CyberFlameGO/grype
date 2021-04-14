package file

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	_  = iota
	KB = 1 << (10 * iota)
	MB
	GB
)

type errZipSlipDetected struct {
	Prefix   string
	JoinArgs []string
}

func (e *errZipSlipDetected) Error() string {
	return fmt.Sprintf("potential zip slip attack: prefix=%q dest=%q", e.Prefix, e.JoinArgs)
}

// safeJoin ensures that any destinations do not resolve to a path above the prefix path.
func safeJoin(prefix string, dest ...string) (string, error) {
	joinResult := filepath.Join(append([]string{prefix}, dest...)...)
	cleanJoinResult := filepath.Clean(joinResult)
	if !strings.HasPrefix(cleanJoinResult, filepath.Clean(prefix)) {
		return "", &errZipSlipDetected{
			Prefix:   prefix,
			JoinArgs: dest,
		}
	}
	// why not return the clean path? the called may not be expected it from what should only be a join operation.
	return joinResult, nil
}

func UnTarGz(dst string, r io.Reader) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil

		case err != nil:
			return err

		case header == nil:
			continue
		}

		target, err := safeJoin(dst, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return fmt.Errorf("failed to mkdir (%s): %w", target, err)
				}
			}

		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to open file (%s): %w", target, err)
			}

			// limit the tar reader to 1GB per file to prevent decompression bomb attacks
			if _, err := io.Copy(f, io.LimitReader(tr, 1*GB)); err != nil {
				return fmt.Errorf("failed to copy file (%s): %w", target, err)
			}

			if err = f.Close(); err != nil {
				return fmt.Errorf("failed to close file (%s): %w", target, err)
			}
		}
	}
}

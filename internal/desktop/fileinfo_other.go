//go:build !unix

package desktop

import (
	"errors"
	"io/fs"
)

func checkPrivateFile(fs.FileInfo, int) error { return errors.New("unsupported platform") }

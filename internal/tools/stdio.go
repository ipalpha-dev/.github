package tools

import (
	"io"
	"os"
)

func stdIO() (io.Reader, io.Writer, io.Writer) { return os.Stdin, os.Stdout, os.Stderr }

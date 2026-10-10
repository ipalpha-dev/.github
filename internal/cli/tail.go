package cli

import (
	"io"
	"os"
	"time"
)

// tailFollow prints a file and keeps printing what is appended (Ctrl+C to stop). Works the same
// on every OS (no `tail -f`).
func tailFollow(path string) error {
	var off int64
	for {
		f, err := os.Open(path)
		if err == nil {
			st, _ := f.Stat()
			if st != nil && st.Size() < off {
				off = 0 // truncated by a restart
			}
			_, _ = f.Seek(off, io.SeekStart)
			n, _ := io.Copy(os.Stdout, f)
			off += n
			f.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
}

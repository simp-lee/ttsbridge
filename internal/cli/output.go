package cli

import (
	"fmt"
	"io"
)

func writeLineIgnoreError(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

func writeFormatIgnoreError(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

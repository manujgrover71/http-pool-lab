package csvw

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type File struct {
	f *os.File
	w *bufio.Writer
}

func Create(path string, header ...string) (*File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	c := &File{
		f: f,
		w: bufio.NewWriter(f),
	}
	fmt.Fprintln(c.w, strings.Join(header, ","))
	return c, nil
}

func (c *File) Write(vals ...any) {
	for i, v := range vals {
		if i > 0 {
			c.w.WriteByte(',')
		}
		fmt.Fprint(c.w, v)
	}
	c.w.WriteByte('\n')
}

func (c *File) Close() error {
	if err := c.w.Flush(); err != nil {
		return err
	}
	return c.f.Close()
}

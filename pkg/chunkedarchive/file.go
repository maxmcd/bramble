package chunkedarchive

import (
	"io"
	"os"
)

func FileArchive(location string, archive string) error {
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	if err := StreamArchive(f, location); err != nil {
		return err
	}
	return f.Close()
}

func FileUnarchive(archive, dest string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	return StreamUnarchive(io.NewSectionReader(f, 0, fi.Size()), dest)
}

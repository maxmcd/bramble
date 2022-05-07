package url2

import (
	"net/url"
	"path"
	"path/filepath"
)

// Join is a url-aware path.Join, it will try and parse the first element as a
// valid url and then join any subsequent paths. If the first element errors
// when attempting to parse the passed elements will be joined with path.Join
func Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	u, err := url.Parse(elem[0])
	if err != nil {
		return path.Join(elem...)
	}
	pathElems := append([]string{u.Path}, elem[1:]...)
	u.Path = path.Join(pathElems...)
	// Preserve trailing slash if passed
	if lastCharOfLastElem(pathElems) == "/" {
		u.Path += "/"
	}
	return u.String()
}

func Rel(basepath, targetpath string) (string, error) {
	u, err := url.Parse(basepath)
	if err != nil {
		return filepath.Rel(basepath, targetpath)
	}
	u.Path, err = filepath.Rel(basepath, targetpath)
	// Preserve trailing slash if passed
	if err == nil && string(targetpath[len(u.Path)-1]) == "/" {
		u.Path += "/"
	}
	return u.String(), nil
}

func lastCharOfLastElem(elems []string) string {
	if len(elems) == 0 {
		return ""
	}
	last := elems[len(elems)-1]
	if len(last) == 0 {
		return ""
	}
	return string(last[len(last)-1])
}

package bramble

import (
	"os"
	"path"
)

func CommonPrefix(paths []string) string {
	sep := byte(os.PathSeparator)
	if len(paths) == 0 {
		return string(sep)
	}

	c := []byte(path.Clean(paths[0]))
	c = append(c, sep)

	for _, v := range paths[1:] {
		v = path.Clean(v) + string(sep)
		if len(v) < len(c) {
			c = c[:len(v)]
		}
		for i := 0; i < len(c); i++ {
			if v[i] != c[i] {
				c = c[:i]
				break
			}
		}
	}

	for i := len(c) - 1; i >= 0; i-- {
		if c[i] == sep {
			c = c[:i+1]
			break
		}
	}

	return string(c)
}

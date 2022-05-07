package s3test

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type PostResponse struct {
	ETag     string
	UploadID string `xml:"UploadId"`
}
type Server struct {
	uploads map[string]*Upload
	lock    sync.Mutex

	server *httptest.Server

	objDir string
}

func (s *Server) Hostname() string {
	return s.server.Listener.Addr().String()
}

func StartServer(t *testing.T, addr string) (s *Server) {
	s = &Server{uploads: map[string]*Upload{}}
	s.objDir = t.TempDir()

	s.server = httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if err := s.handler(rw, r); err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(rw, "error: "+err.Error())
		}
	}))
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	s.server.Listener = listener
	s.server.Start()
	return s
}

func (s *Server) handler(rw http.ResponseWriter, r *http.Request) (err error) {
	b, _ := httputil.DumpRequest(r, false)
	fmt.Println(string(b))
	uploadID := r.URL.Query().Get("uploadId")
	switch r.Method {
	case http.MethodGet:
		loc := filepath.Join(s.objDir, strings.TrimPrefix(r.URL.Path, "/bramble"))
		f, err := os.Open(loc)
		if err == os.ErrNotExist {
			rw.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(rw, "not found")
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := io.Copy(rw, f); err != nil {
			return err
		}
		return f.Close()
	case http.MethodPost:
		var pr PostResponse
		if uploadID == "" {
			// New
			pr.UploadID, _ = s.newUpload(strings.TrimPrefix(r.URL.Path, "/bramble"))
		} else {
			// Existing
			s.lock.Lock()
			upload, found := s.uploads[uploadID]
			s.lock.Unlock()
			if !found {
				return errors.New("upload not found ")
			}
			if err := upload.file.Close(); err != nil {
				return err
			}
			pr.UploadID = uploadID
			pr.ETag = fmt.Sprintf("%x", upload.md5OfParts.Sum(nil))
		}

		if err := xml.NewEncoder(rw).Encode(pr); err != nil {
			return err
		}

	case http.MethodPut:
		if uploadID == "" {
			return errors.New("no upload id")
		}
		partNumberStr := r.URL.Query().Get("partNumber")
		hasPartNumber := partNumberStr != ""
		var partNumber int
		if hasPartNumber {
			partNumber, err = strconv.Atoi(partNumberStr)
			if err != nil {
				panic(err)
			}
		}
		s.lock.Lock()
		upload, found := s.uploads[uploadID]
		s.lock.Unlock()
		if !found {
			return errors.New("upload not found ")
		}
		if hasPartNumber {
			upload.waitForPart(partNumber)
			defer upload.incrementPartNumber()
		}

		m := md5.New()
		_, _ = io.Copy(io.MultiWriter(m, upload.file), r.Body)
		upload.md5OfParts.Write(m.Sum(nil))
		rw.Header().Add("etag", fmt.Sprintf(`"%s"`, hex.EncodeToString(m.Sum(nil))))
	default:
		panic("")
	}
	return nil
}

func (s *Server) newUpload(path string) (id string, u *Upload) {
	u = &Upload{}
	id = fmt.Sprint(time.Now().UnixNano())
	objPath := filepath.Join(s.objDir, path)
	_ = os.MkdirAll(filepath.Dir(objPath), 0755)
	f, err := os.Create(objPath)
	if err != nil {
		panic(err)
	}
	u.file = f

	u.partNumber = 1
	u.md5OfParts = md5.New()
	s.lock.Lock()
	s.uploads[id] = u
	s.lock.Unlock()
	return
}

type Upload struct {
	md5OfParts hash.Hash
	partNumber int
	lock       sync.Mutex

	file *os.File
}

func (u *Upload) waitForPart(partNumber int) {
	defer u.lock.Unlock()
	for {
		u.lock.Lock()
		if u.partNumber == partNumber {
			return
		}
		u.lock.Unlock()
		time.Sleep(time.Millisecond * 5)
	}
}

func (u *Upload) incrementPartNumber() {
	u.lock.Lock()
	u.partNumber++
	u.lock.Unlock()
}

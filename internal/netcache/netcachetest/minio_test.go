package netcachetest

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinio(t *testing.T) {
	client := StartMinio(t)

	fakeFile := make([]byte, 1e7)
	if _, err := rand.Read(fakeFile); err != nil {
		t.Fatal(err)
	}

	key := "output/testfile@+-$%^"
	{
		// put
		writer, err := client.Put(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(writer, bytes.NewBuffer(fakeFile)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}

	{
		// get
		reader, err := client.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, reader); err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		require.Equal(t, fakeFile, buf.Bytes())
	}
}

package resources_test

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/apex/log"
	"github.com/ooni/probe-engine/resources"
)

func TestEnsure(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	tempdir, err := ioutil.TempDir("", "ooniprobe-engine-resources-test")
	if err != nil {
		t.Fatal(err)
	}
	client := resources.Client{
		HTTPClient: http.DefaultClient,
		Logger:     log.Log,
		UserAgent:  "ooniprobe-engine/0.1.0",
		WorkDir:    tempdir,
	}
	err = client.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// the second round should be idempotent
	err = client.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func TestEnsureFailure(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	tempdir, err := ioutil.TempDir("", "ooniprobe-engine-resources-test")
	if err != nil {
		t.Fatal(err)
	}
	client := resources.Client{
		HTTPClient: http.DefaultClient,
		Logger:     log.Log,
		UserAgent:  "ooniprobe-engine/0.1.0",
		WorkDir:    tempdir,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.Ensure(ctx)
	if err == nil {
		t.Fatal("expected an error here")
	}
}

func TestEnsureFailAllComparisons(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	tempdir, err := ioutil.TempDir("", "ooniprobe-engine-resources-test")
	if err != nil {
		t.Fatal(err)
	}
	client := resources.Client{
		HTTPClient: http.DefaultClient,
		Logger:     log.Log,
		UserAgent:  "ooniprobe-engine/0.1.0",
		WorkDir:    tempdir,
	}
	// run once to download the resource once
	err = client.EnsureForSingleResource(
		context.Background(), "ca-bundle.pem", resources.ResourceInfo{
			URLPath:  "/releases/download/20190822135402/ca-bundle.pem.gz",
			GzSHA256: "d5a6aa2290ee18b09cc4fb479e2577ed5ae66c253870ba09776803a5396ea3ab",
			SHA256:   "cb2eca3fbfa232c9e3874e3852d43b33589f27face98eef10242a853d83a437a",
		}, func(left, right string) bool {
			return left == right
		},
		gzip.NewReader, ioutil.ReadAll,
	)
	if err != nil {
		t.Fatal(err)
	}
	// re-run with broken comparison operator so that we should
	// first redownload and then fail for invalid SHA256.
	err = client.EnsureForSingleResource(
		context.Background(), "ca-bundle.pem", resources.ResourceInfo{
			URLPath:  "/releases/download/20190822135402/ca-bundle.pem.gz",
			GzSHA256: "d5a6aa2290ee18b09cc4fb479e2577ed5ae66c253870ba09776803a5396ea3ab",
			SHA256:   "cb2eca3fbfa232c9e3874e3852d43b33589f27face98eef10242a853d83a437a",
		}, func(left, right string) bool {
			return false // comparison for equality always fails
		},
		gzip.NewReader, ioutil.ReadAll,
	)
	if err == nil {
		t.Fatal("expected an error here")
	}
}

func TestEnsureFailGzipNewReader(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	tempdir, err := ioutil.TempDir("", "ooniprobe-engine-resources-test")
	if err != nil {
		t.Fatal(err)
	}
	client := resources.Client{
		HTTPClient: http.DefaultClient,
		Logger:     log.Log,
		UserAgent:  "ooniprobe-engine/0.1.0",
		WorkDir:    tempdir,
	}
	err = client.EnsureForSingleResource(
		context.Background(), "ca-bundle.pem", resources.ResourceInfo{
			URLPath:  "/releases/download/20190822135402/ca-bundle.pem.gz",
			GzSHA256: "d5a6aa2290ee18b09cc4fb479e2577ed5ae66c253870ba09776803a5396ea3ab",
			SHA256:   "cb2eca3fbfa232c9e3874e3852d43b33589f27face98eef10242a853d83a437a",
		}, func(left, right string) bool {
			return left == right
		},
		func(r io.Reader) (*gzip.Reader, error) {
			return nil, errors.New("mocked error")
		},
		ioutil.ReadAll,
	)
	if err == nil {
		t.Fatal("expected an error here")
	}
}

func TestEnsureFailIoUtilReadAll(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	tempdir, err := ioutil.TempDir("", "ooniprobe-engine-resources-test")
	if err != nil {
		t.Fatal(err)
	}
	client := resources.Client{
		HTTPClient: http.DefaultClient,
		Logger:     log.Log,
		UserAgent:  "ooniprobe-engine/0.1.0",
		WorkDir:    tempdir,
	}
	err = client.EnsureForSingleResource(
		context.Background(), "ca-bundle.pem", resources.ResourceInfo{
			URLPath:  "/releases/download/20190822135402/ca-bundle.pem.gz",
			GzSHA256: "d5a6aa2290ee18b09cc4fb479e2577ed5ae66c253870ba09776803a5396ea3ab",
			SHA256:   "cb2eca3fbfa232c9e3874e3852d43b33589f27face98eef10242a853d83a437a",
		}, func(left, right string) bool {
			return left == right
		},
		gzip.NewReader, func(r io.Reader) ([]byte, error) {
			return nil, errors.New("mocked error")
		},
	)
	if err == nil {
		t.Fatal("expected an error here")
	}
}

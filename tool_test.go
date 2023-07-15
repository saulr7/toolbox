package toolbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
)

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewTestClient(fn RoundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func TestTool_PushJSONToRemote(t *testing.T) {

	client := NewTestClient(func(req *http.Request) *http.Response {
		// Test request params
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewBufferString("ok")),
			Header:     make(http.Header),
		}
	})

	var tools Tools

	var foo struct {
		Bar string `json:"bar"`
	}
	foo.Bar = "baar"

	_, _, err := tools.PushJSONToRemote("/", foo, client)

	if err != nil {
		t.Error(err)
	}

}

func TestTools_RandomString(t *testing.T) {
	var testTools Tools

	s := testTools.RandomString(10)

	if len(s) != 10 {
		t.Error("expected 10 characters")
	}

}

var uploadTest = []struct {
	name          string
	allowedTypes  []string
	renameFile    bool
	errorExpected bool
}{
	{name: "allowed no rename", allowedTypes: []string{"image/jpg", "image/png"}, renameFile: false, errorExpected: false},
	{name: "allowed rename", allowedTypes: []string{"image/jpg", "image/png"}, renameFile: true, errorExpected: false},
	{name: "not allowed", allowedTypes: []string{"image/jpg"}, renameFile: false, errorExpected: true},
}

func TestTools_UploadFiles(t *testing.T) {

	for _, e := range uploadTest {

		//setup a pipe to avoid buffering
		pr, pw := io.Pipe()

		writer := multipart.NewWriter(pw)

		wg := sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer writer.Close()
			defer wg.Done()

			// create the form data field
			part, err := writer.CreateFormFile("file", "./testdata/img.png")
			if err != nil {
				t.Error(err)
			}

			f, err := os.Open("./testdata/img.png")

			if err != nil {
				t.Error(err)
			}

			defer f.Close()

			img, _, err := image.Decode(f)

			if err != nil {
				t.Error("error decoding image", err)
			}
			err = png.Encode(part, img)

			if err != nil {
				t.Error(err)
			}

		}()

		//read from the pipe which receives data

		request := httptest.NewRequest("POTS", "/", pr)
		request.Header.Add("Content-Type", writer.FormDataContentType())

		var testTools Tools
		testTools.AllowedFileTypes = e.allowedTypes

		uploadedFiles, err := testTools.UploadFiles(request, "./testdata/upload/", e.renameFile)

		if err != nil && !e.errorExpected {
			t.Error(err)
		}

		if !e.errorExpected {
			if _, err := os.Stat(fmt.Sprintf("./testdata/upload/%s", uploadedFiles[0].NewFileName)); os.IsNotExist(err) {
				t.Errorf("%s: expected file to exists", e.name)
			}
			//clean up
			_ = os.Remove(fmt.Sprintf("./testdata/upload/%s", uploadedFiles[0].NewFileName))
		}

		if !e.errorExpected && err != nil {
			t.Error(err)
		}

		wg.Wait()

	}

}

func TestTools_UploadOneFile(t *testing.T) {

	//setup a pipe to avoid buffering
	pr, pw := io.Pipe()

	writer := multipart.NewWriter(pw)

	go func() {
		defer writer.Close()

		// create the form data field
		part, err := writer.CreateFormFile("file", "./testdata/img.png")
		if err != nil {
			t.Error(err)
		}

		f, err := os.Open("./testdata/img.png")

		if err != nil {
			t.Error(err)
		}

		defer f.Close()

		img, _, err := image.Decode(f)

		if err != nil {
			t.Error("error decoding image", err)
		}
		err = png.Encode(part, img)

		if err != nil {
			t.Error(err)
		}

	}()

	//read from the pipe which receives data
	request := httptest.NewRequest("POTS", "/", pr)
	request.Header.Add("Content-Type", writer.FormDataContentType())

	var testTools Tools

	uploadedFile, err := testTools.UploadOneFile(request, "./testdata/upload/", true)

	if err != nil {
		t.Error(err)
	}

	if _, err := os.Stat(fmt.Sprintf("./testdata/upload/%s", uploadedFile.NewFileName)); os.IsNotExist(err) {
		t.Errorf("expected file to exists")
	}
	//clean up
	_ = os.Remove(fmt.Sprintf("./testdata/upload/%s", uploadedFile.NewFileName))

}
func TestTools_CreateDirIfNotExists(t *testing.T) {

	var testTool Tools

	err := testTool.CreateDirIfNotExist("./testdata/myDir")

	if err != nil {
		t.Error(err)
	}

	err = testTool.CreateDirIfNotExist("./testdata/myDir")

	if err != nil {
		t.Error(err)
	}

	os.Remove("./testdata/myDir")
}

func TestTools_Slugify(t *testing.T) {

	var slugTest = []struct {
		name          string
		s             string
		expected      string
		errorExpected bool
	}{
		{name: "valid string", s: "now is the time", expected: "now-is-the-time", errorExpected: false},
		{name: "empty string", s: "", expected: "he", errorExpected: true},
		{name: "complex string", s: "Now is the time for all GOOD+=", expected: "now-is-the-time-for-all-good", errorExpected: false},
		{name: "Japanase string", s: "ハローワールド", expected: "", errorExpected: true},
		{name: "Japanase and roman strings", s: "helloハローワールド", expected: "hello", errorExpected: false},
	}

	var toolsTest Tools

	for _, e := range slugTest {

		slug, err := toolsTest.Slugify(e.s)

		if !e.errorExpected && err != nil {
			t.Error(err)
		}

		if !e.errorExpected && slug != e.expected {
			t.Errorf("%s: bad slug", e.name)
		}

	}

}

func TestTool_DownloadStaticFile(t *testing.T) {

	rr := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/", nil)

	var testtools Tools

	testtools.DownloadStaticFiles(rr, req, "./testdata", "tanjiro.jpg", "Tanjiro.jpg")

	res := rr.Result()
	defer res.Body.Close()

	sizeStr := res.Header["Content-Length"][0]

	size, err := strconv.Atoi(sizeStr)

	if err != nil {
		t.Error(err)
	}

	if size != 161289 {
		t.Error("wrong content length of", size)
	}

	if res.Header["Content-Disposition"][0] != "attachment; filename\"Tanjiro.jpg\"" {
		t.Error("wrong content disposition")
	}

	_, err = ioutil.ReadAll(res.Body)

	if err != nil {
		t.Error(err)
	}

}

var jsonTests = []struct {
	name          string
	json          string
	errorExpected bool
	maxSize       int
	allowUnkown   bool
}{
	{name: "Good json", json: `{"foo":"bar"}`, errorExpected: false, maxSize: 1024, allowUnkown: false},
	{name: "Badly json", json: `{"foo":}`, errorExpected: true, maxSize: 1024, allowUnkown: false},
	{name: "Incorrect type", json: `{"foo":2}`, errorExpected: true, maxSize: 1024, allowUnkown: false},
	{name: "tow json files", json: `{"foo":"2"}{"a":"b"}`, errorExpected: true, maxSize: 1024, allowUnkown: false},
	{name: "empty", json: ``, errorExpected: true, maxSize: 1024, allowUnkown: false},
	{name: "syntax error", json: `{"foo":1"}`, errorExpected: true, maxSize: 1024, allowUnkown: false},
	{name: "unknown field", json: `{"foo2":"1"}`, errorExpected: true, maxSize: 1024, allowUnkown: false},
	{name: "allow unknown fields", json: `{"foo":"1","foo2":"1"}`, errorExpected: false, maxSize: 1024, allowUnkown: true},
	{name: "missing field name", json: `{jack:"1"}`, errorExpected: true, maxSize: 1024, allowUnkown: true},
	{name: "file too large", json: `{"foo":"1"}`, errorExpected: true, maxSize: 2, allowUnkown: true},
	{name: "not a json", json: `Hello world`, errorExpected: true, maxSize: 1024, allowUnkown: true},
}

func TestTools_ReadJSON(t *testing.T) {

	var tools Tools

	for _, e := range jsonTests {

		//max size
		tools.MaxJSONSize = e.maxSize

		//alllow unkown fields
		tools.AllowUnkownFields = e.allowUnkown

		//decoded
		var decodedJSON struct {
			Foo string `json:"foo"`
		}

		req, err := http.NewRequest("POST", "/", bytes.NewReader([]byte(e.json)))

		if err != nil {
			t.Error(err)
		}

		rr := httptest.NewRecorder()

		err = tools.ReadJSON(rr, req, &decodedJSON)

		if e.errorExpected && err == nil {
			t.Errorf("%s: expected error but none receive", e.name)
		}

		if !e.errorExpected && err != nil {
			t.Errorf("%s: error not expected but one received", e.name)
		}

		req.Body.Close()

	}

}

func TestTools_WriteJson(t *testing.T) {

	var tools Tools

	rr := httptest.NewRecorder()

	payload := JSONResponse{
		Error:   false,
		Message: "foo",
	}

	headers := make(http.Header)
	headers.Add("FOOT", "BAR")

	err := tools.WriteJSON(rr, http.StatusOK, payload, headers)

	if err != nil {
		t.Error(err)
	}

}

func TestTool_ErrorJSON(t *testing.T) {

	var tools Tools

	rr := httptest.NewRecorder()

	err := tools.ErrorJSON(rr, errors.New("some error"), http.StatusServiceUnavailable)

	if err != nil {
		t.Error(err)
	}

	var payload JSONResponse
	dec := json.NewDecoder(rr.Body)

	err = dec.Decode(&payload)

	if err != nil {
		t.Error(err)
	}

	if !payload.Error {
		t.Error("expected set to true error but was false")
	}

	if rr.Code != http.StatusServiceUnavailable {
		t.Error("wrong status code")
	}

}

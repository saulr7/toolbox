package toolbox

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const randomStringSource = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_+"

// Tools is the type used to instatiate this module
type Tools struct {
	MaxFileSize       int
	AllowedFileTypes  []string
	MaxJSONSize       int
	AllowUnkownFields bool
}

// RandomString returns a string of random characters of length n
func (t *Tools) RandomString(n int) string {

	s, r := make([]rune, n), []rune(randomStringSource)

	for i := range s {
		p, _ := rand.Prime(rand.Reader, len(r))
		x, y := p.Uint64(), uint64(len(r))
		s[i] = r[x%y]
	}

	return string(s)

}

// UploadedFile is an struct used to save information
type UploadedFile struct {
	NewFileName      string
	OriginalFileName string
	FileSize         int64
}

func (t *Tools) UploadOneFile(r *http.Request, uploadDir string, rename ...bool) (*UploadedFile, error) {

	renameFile := true

	if len(rename) > 0 {
		renameFile = rename[0]
	}

	files, err := t.UploadFiles(r, uploadDir, renameFile)

	if err != nil {
		return nil, err
	}

	return files[0], nil
}

func (t *Tools) UploadFiles(r *http.Request, uploadDir string, rename ...bool) ([]*UploadedFile, error) {

	renameFile := true

	if len(rename) > 0 {
		renameFile = rename[0]
	}

	var uploadedFiles []*UploadedFile

	if t.MaxFileSize == 0 {
		t.MaxFileSize = 1024 * 1024 * 1024
	}

	err := t.CreateDirIfNotExist(uploadDir)
	if err != nil {
		return nil, err
	}

	err = r.ParseMultipartForm(int64(t.MaxFileSize))

	if err != nil {
		return nil, errors.New("the uploaded file is too big")
	}

	for _, fHeaders := range r.MultipartForm.File {

		for _, hdr := range fHeaders {
			uploadedFiles, err := func(uploadeFiles []*UploadedFile) ([]*UploadedFile, error) {

				var uploadedFile UploadedFile
				infile, err := hdr.Open()

				if err != nil {
					return nil, err
				}
				defer infile.Close()

				buff := make([]byte, 512)
				_, err = infile.Read(buff)
				if err != nil {
					return nil, err
				}

				//check to see if file type is permitted
				allowed := false
				fileType := http.DetectContentType(buff)

				if len(t.AllowedFileTypes) > 0 {
					for _, x := range t.AllowedFileTypes {
						if strings.EqualFold(fileType, x) {
							allowed = true
						}
					}
				} else {
					allowed = true
				}

				if !allowed {
					return nil, errors.New("the uploaded file type is not permitted")
				}
				_, err = infile.Seek(0, 0)

				if err != nil {
					return nil, err
				}

				if renameFile {
					uploadedFile.NewFileName = fmt.Sprintf("%s%s", t.RandomString(25), filepath.Ext(hdr.Filename))
				} else {
					uploadedFile.NewFileName = hdr.Filename
				}
				uploadedFile.OriginalFileName = hdr.Filename

				var outfile *os.File
				defer outfile.Close()

				if outfile, err := os.Create(filepath.Join(uploadDir, uploadedFile.NewFileName)); err != nil {
					return nil, err
				} else {
					fileSize, err := io.Copy(outfile, infile)

					if err != nil {
						return nil, err
					}

					uploadedFile.FileSize = fileSize
				}
				uploadedFiles = append(uploadedFiles, &uploadedFile)
				return uploadeFiles, nil

			}(uploadedFiles)

			if err != nil {
				return uploadedFiles, err
			}
		}

	}

	return uploadedFiles, nil
}

// CreateDirIfNotExist creates a directory and all necessary parents
func (t *Tools) CreateDirIfNotExist(path string) error {

	const mode = 0755

	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = os.MkdirAll(path, mode)

		if err != nil {
			return err
		}
	}
	return nil
}

// Slugify is a very simple way to create slug from string
func (t *Tools) Slugify(s string) (string, error) {

	if len(s) == 0 {
		return "", errors.New("empty string")
	}

	re := regexp.MustCompile(`[^a-z\d]+`)

	slug := strings.Trim(re.ReplaceAllString(strings.ToLower(s), "-"), "-")

	if len(slug) == 0 {
		return "", errors.New("after removing character, slug is zero leng")

	}
	return slug, nil

}

// DownloadStaticFiles it downloads a file and tries to force the browser to download it instead of displaying it
func (t *Tools) DownloadStaticFiles(w http.ResponseWriter, r *http.Request, p, file, displayName string) {

	fp := path.Join(p, file)

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename\"%s\"", displayName))

	http.ServeFile(w, r, fp)

}

// JSONResponse is the type used to send JSON around
type JSONResponse struct {
	Error   bool        `json:"error"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ReadJSON tries to read a body of a request and convert it
func (t *Tools) ReadJSON(w http.ResponseWriter, r *http.Request, data interface{}) error {

	maxBytes := 1024 * 1024

	if t.MaxJSONSize != 0 {
		maxBytes = t.MaxJSONSize
	}

	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

	dec := json.NewDecoder(r.Body)

	if !t.AllowUnkownFields {
		dec.DisallowUnknownFields()
	}
	err := dec.Decode(data)

	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at: %d)", unmarshalTypeError.Offset)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		case strings.HasPrefix(err.Error(), "json:unkown field"):
			fieldName := strings.TrimPrefix(err.Error(), "json:unkown field")
			return fmt.Errorf("body containes unknown key %s", fieldName)
		case err.Error() == "http: request body too large":
			return fmt.Errorf("body must not be larger than %d bytes", maxBytes)
		case errors.As(err, &invalidUnmarshalError):
			return fmt.Errorf("error unmarshaling JSON: %s", err)
		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})

	if err != io.EOF {
		return errors.New("body must contain only one JSON value")
	}

	return nil

}

// WriteJSON take a respon, status code and data and writes json
func (t *Tools) WriteJSON(w http.ResponseWriter, status int, data interface{}, headers ...http.Header) error {

	out, err := json.Marshal(data)

	if err != nil {
		return err
	}

	if len(headers) > 0 {
		for key, value := range headers[0] {
			w.Header()[key] = value
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	_, err = w.Write(out)

	return err
}

// ErrorJSON takes an error and optinally an status code and sends a JSON error message
func (t *Tools) ErrorJSON(w http.ResponseWriter, err error, status ...int) error {

	statusCode := http.StatusBadRequest

	if len(status) > 0 {
		statusCode = status[0]
	}

	var payload JSONResponse

	payload.Error = true
	payload.Message = err.Error()

	return t.WriteJSON(w, statusCode, payload)

}

// PushJSONToRemote post data to the specify url and returns the response, code and error if any
func (t *Tools) PushJSONToRemote(url string, data interface{}, client ...*http.Client) (*http.Response, int, error) {

	//create JSON
	jsonData, err := json.Marshal(data)

	if err != nil {
		return nil, 0, err
	}

	//http client
	httpClient := &http.Client{}

	if len(client) > 0 {
		httpClient = client[0]
	}

	//build request
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))

	if err != nil {
		return nil, 0, err
	}

	request.Header.Set("Content-Type", "application/json")

	//call remote url
	response, err := httpClient.Do(request)

	if err != nil {
		return nil, 0, err
	}

	defer response.Body.Close()

	//send response
	return response, response.StatusCode, nil

}

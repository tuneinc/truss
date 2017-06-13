package templates

// ClientEncodeTemplate is the template for generating the client-side encoding
// function for a particular Binding.
var ClientEncodeTemplate = `
{{- with $binding := . -}}
	// EncodeHTTP{{$binding.Label}}Request is a transport/http.EncodeRequestFunc
	// that encodes a {{ToLower $binding.Parent.Name}} request into the various portions of
	// the http request (path, query, and body).
	func EncodeHTTP{{$binding.Label}}Request(_ context.Context, r *http.Request, request interface{}) error {
		strval := ""
		_ = strval
		req := request.(*pb.{{GoName $binding.Parent.RequestType}})
		_ = req

		r.Header.Set("transport", "HTTPJSON")
		r.Header.Set("request-url", r.URL.Path)

		// Set the path parameters
		path := strings.Join([]string{
		{{- range $section := $binding.PathSections}}
			{{$section}},
		{{- end}}
		}, "/")
		u, err := url.Parse(path)
		if err != nil {
			return errors.Wrapf(err, "couldn't unmarshal path %q", path)
		}
		r.URL.RawPath = u.RawPath
		r.URL.Path = u.Path

		// Set the query parameters
		values := r.URL.Query()
		var tmp []byte
		_ = tmp
		{{- range $field := $binding.Fields }}
			{{- if eq $field.Location "query"}}
				{{if or (not $field.IsBaseType) $field.Repeated}}
					tmp, err = json.Marshal(req.{{$field.CamelName}})
					if err != nil {
						return errors.Wrap(err, "failed to marshal req.{{$field.CamelName}}")
					}
					strval = string(tmp)
					values.Add("{{$field.QueryParamName}}", strval)
				{{else}}
					values.Add("{{$field.QueryParamName}}", fmt.Sprint(req.{{$field.CamelName}}))
				{{- end }}
			{{- end }}
		{{- end}}

		r.URL.RawQuery = values.Encode()

		// Set the body parameters
		var buf bytes.Buffer
		toRet := request.(*pb.{{GoName $binding.Parent.RequestType}})
		{{- range $field := $binding.Fields -}}
			{{if eq $field.Location "body"}}
				{{/* Only set the fields which should be in the body, so all
				others will be omitted due to emptiness */}}
				toRet.{{$field.CamelName}} = req.{{$field.CamelName}}
			{{end}}
		{{- end }}
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(toRet); err != nil {
			return errors.Wrapf(err, "couldn't encode body as json %v", toRet)
		}
		r.Body = ioutil.NopCloser(&buf)
		return nil
	}
{{- end -}}
`

var ClientTemplate = `
// Code generated by truss.
// Rerunning truss will overwrite this file.
// DO NOT EDIT!
// Version: {{.Version}}
// Version Date: {{.VersionDate}}

// Package http provides an HTTP client for the {{.Service.Name}} service.
package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
	"github.com/pkg/errors"
	"golang.org/x/net/context"

	// This Service
	"{{.ImportPath -}} /svc"
	pb "{{.PBImportPath -}}"
)

var (
	_ = endpoint.Chain
	_ = httptransport.NewClient
	_ = fmt.Sprint
	_ = bytes.Compare
	_ = ioutil.NopCloser
)

// New returns a service backed by an HTTP server living at the remote
// instance. We expect instance to come from a service discovery system, so
// likely of the form "host:port".
func New(instance string, options ...ClientOption) (pb.{{GoName .Service.Name}}Server, error) {
	var cc clientConfig

	for _, f := range options {
		err := f(&cc)
		if err != nil {
			return nil, errors.Wrap(err, "cannot apply option") }
	}

	{{ if .HTTPHelper.Methods }}
		clientOptions := []httptransport.ClientOption{
			httptransport.ClientBefore(
				contextValuesToHttpHeaders(cc.headers)),
		}
	{{ end }}

	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	u, err := url.Parse(instance)
	if err != nil {
		return nil, err
	}
	_ = u

	{{if not .HTTPHelper.Methods -}}
		panic("No HTTP Endpoints, this client will not work, define bindings in your proto definition")
	{{- end}}

	{{range $method := .HTTPHelper.Methods}}
		{{ if $method.Bindings -}}
			{{ with $binding := index $method.Bindings 0 -}}
				var {{$binding.Label}}Endpoint endpoint.Endpoint
				{
					{{$binding.Label}}Endpoint = httptransport.NewClient(
						"{{$binding.Verb}}",
						copyURL(u, "{{$binding.BasePath}}"),
						EncodeHTTP{{$binding.Label}}Request,
						DecodeHTTP{{$method.Name}}Response,
						clientOptions...,
					).Endpoint()
				}
			{{- end}}
		{{- end}}
	{{- end}}

	return svc.Endpoints{
	{{range $method := .HTTPHelper.Methods -}}
		{{ if $method.Bindings -}}
			{{ with $binding := index $method.Bindings 0 -}}
				{{$method.Name}}Endpoint:    {{$binding.Label}}Endpoint,
			{{end}}
		{{- end}}
	{{- end}}
	}, nil
}

func copyURL(base *url.URL, path string) *url.URL {
	next := *base
	next.Path = path
	return &next
}

type clientConfig struct {
	headers []string
}

// ClientOption is a function that modifies the client config
type ClientOption func(*clientConfig) error

// CtxValuesToSend configures the http client to pull the specified keys out of
// the context and add them to the http request as headers.  Note that keys
// will have net/http.CanonicalHeaderKey called on them before being send over
// the wire and that is the form they will be available in the server context.
func CtxValuesToSend(keys ...string) ClientOption {
	return func(o *clientConfig) error {
		o.headers = keys
		return nil
	}
}

func contextValuesToHttpHeaders(keys []string) httptransport.RequestFunc {
	return func(ctx context.Context, r *http.Request) context.Context {
		for _, k := range keys {
			if v, ok := ctx.Value(k).(string); ok {
				r.Header.Set(k, v)
			}
		}

		return ctx
	}
}

// HTTP Client Decode
{{range $method := .HTTPHelper.Methods}}
	// DecodeHTTP{{$method.Name}}Response is a transport/http.DecodeResponseFunc that decodes
	// a JSON-encoded {{GoName $method.ResponseType}} response from the HTTP response body.
	// If the response has a non-200 status code, we will interpret that as an
	// error and attempt to decode the specific error message from the response
	// body. Primarily useful in a client.
	func DecodeHTTP{{$method.Name}}Response(_ context.Context, r *http.Response) (interface{}, error) {
		buf, err := ioutil.ReadAll(r.Body)
		if len(buf) == 0 {
			return nil, errors.New("response http body empty")
		}

		if err != nil {
			return nil, errors.Wrap(err, "cannot read http body")
		}

		if r.StatusCode != http.StatusOK {
			return nil, errors.Wrapf(errorDecoder(buf), "status code: '%d'", r.StatusCode)
		}

		var resp pb.{{GoName $method.ResponseType}}
		if err = json.Unmarshal(buf, &resp); err != nil {
			return nil, errorDecoder(buf)
		}

		return &resp, nil
	}
{{end}}

// HTTP Client Encode
{{range $method := .HTTPHelper.Methods}}
	{{range $binding := $method.Bindings}}
		{{$binding.GenClientEncode}}
	{{end}}
{{end}}

func errorDecoder(buf []byte) error {
	var w errorWrapper
	if err := json.Unmarshal(buf, &w); err != nil {
		const size = 8196
		if len(buf) > size {
			buf = buf[:size]
		}
		return fmt.Errorf("response body '%s': cannot parse non-json request body", buf)
	}

	return errors.New(w.Error)
}

type errorWrapper struct {
	Error string ` + "`json:\"error\"`\n" + `
}
`

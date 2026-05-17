package testutils

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func RequestWithRouteParam(method, path, key, value string, body io.Reader) *http.Request{
	req := httptest.NewRequest(method, path, body)
	return withRouteParam(req, map[string]string{key:value})
}

func withRouteParam(req *http.Request, param map[string]string) *http.Request{
	routeCtx := chi.NewRouteContext()
	for k,v := range param{
		routeCtx.URLParams.Add(k,v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func DecodeJSON(t *testing.T, body io.Reader, v any){
	t.Helper()
	if err := json.NewDecoder(body).Decode(v); err!=nil{
		t.Fatalf("failed to decode json: %v", err)
	}
}
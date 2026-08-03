package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/serverledge-faas/serverledge/internal/cache"
	"github.com/serverledge-faas/serverledge/internal/config"
	"github.com/serverledge-faas/serverledge/internal/function"
	"github.com/serverledge-faas/serverledge/utils"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "serverledge-api-etcd-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	cfg := embed.NewConfig()
	cfg.Dir = tempDir
	cfg.LogOutputs = []string{"/dev/null"}
	cfg.ListenClientUrls = []url.URL{mustParseURL(fmt.Sprintf("http://127.0.0.1:%d", freeTCPPort()))}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.ListenPeerUrls = []url.URL{mustParseURL(fmt.Sprintf("http://127.0.0.1:%d", freeTCPPort()))}
	cfg.AdvertisePeerUrls = cfg.ListenPeerUrls
	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	etcd, err := embed.StartEtcd(cfg)
	if err != nil {
		panic(err)
	}
	defer etcd.Close()

	select {
	case <-etcd.Server.ReadyNotify():
	case <-time.After(30 * time.Second):
		etcd.Server.Stop()
		panic("embedded etcd took too long to start")
	}

	viper.Set(config.ETCD_ADDRESS, cfg.ListenClientUrls[0].Host)
	_, err = utils.GetEtcdClient()
	if err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func mustParseURL(raw string) url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *u
}

func freeTCPPort() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func resetFunctionRegistry(t *testing.T) {
	t.Helper()
	cache.Instance = nil
	cli, err := utils.GetEtcdClient()
	if err != nil {
		t.Fatalf("GetEtcdClient() error = %v", err)
	}
	if _, err := cli.Delete(context.Background(), "/function", clientv3.WithPrefix()); err != nil {
		t.Fatalf("clear functions: %v", err)
	}
}

func apiTestFunction(name string) function.Function {
	return function.Function{
		Name:            name,
		Runtime:         "python314",
		MemoryMB:        128,
		CPUDemand:       0.1,
		MaxConcurrency:  1,
		Handler:         name + ".handler",
		TarFunctionCode: "dGVzdA==",
		SupportedArchs:  []string{"amd64", "arm64"},
		IsDefault:       true,
		SpeedUp:         1,
	}
}

func apiTestVariant(name, base string, speedup, utility float64) function.Function {
	f := apiTestFunction(name)
	f.IsDefault = false
	f.DefaultFunction = base
	f.SpeedUp = speedup
	f.Utility = utility
	return f
}

func TestVariantFunctionAPISerialization(t *testing.T) {
	payload := []byte(`{
		"Name":"api-base-light",
		"Runtime":"python314",
		"MemoryMB":128,
		"CPUDemand":0.1,
		"MaxConcurrency":1,
		"Handler":"api_base_light.handler",
		"SupportedArchs":["amd64","arm64"],
		"IsDefault":false,
		"DefaultFunction":"api-base",
		"SpeedUp":2.0,
		"Utility":0.75
	}`)

	var f function.Function
	if err := json.Unmarshal(payload, &f); err != nil {
		t.Fatalf("unmarshal API payload: %v", err)
	}
	if err := f.ValidateVariantMetadata(); err != nil {
		t.Fatalf("ValidateVariantMetadata() error = %v", err)
	}
	if f.Name != "api-base-light" || f.DefaultFunction != "api-base" {
		t.Fatalf("variant association = name %q default %q", f.Name, f.DefaultFunction)
	}
	if f.Utility != 0.75 || f.SpeedUp != 2 {
		t.Fatalf("quality metadata = utility %v speedup %v", f.Utility, f.SpeedUp)
	}
	if len(f.SupportedArchs) != 2 || f.SupportedArchs[0] != "amd64" || f.SupportedArchs[1] != "arm64" {
		t.Fatalf("SupportedArchs = %#v", f.SupportedArchs)
	}
}

func TestInvocationResponseJSONIncludesIsDefault(t *testing.T) {
	payload, err := json.Marshal(function.Response{
		Success: true,
		ExecutionReport: function.ExecutionReport{
			Result:    "ok",
			IsDefault: true,
			Duration:  0.1,
		},
	})
	if err != nil {
		t.Fatalf("marshal invocation response: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal invocation response: %v", err)
	}
	if decoded["isDefault"] != true {
		t.Fatalf("isDefault = %#v in %s, want true", decoded["isDefault"], payload)
	}
	if _, ok := decoded["IsDefault"]; ok {
		t.Fatalf("response contains unexpected IsDefault field: %s", payload)
	}
	if decoded["Result"] != "ok" || decoded["Success"] != true {
		t.Fatalf("existing response fields changed: %#v", decoded)
	}
}

func TestCreateFunctionAcceptsBackwardCompatiblePlainPayload(t *testing.T) {
	resetFunctionRegistry(t)

	e := echo.New()
	payload := []byte(`{
		"Name":"plain-api",
		"Runtime":"python314",
		"MemoryMB":128,
		"CPUDemand":0.1,
		"MaxConcurrency":1,
		"Handler":"plain.handler",
		"TarFunctionCode":"dGVzdA=="
	}`)
	req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/create")

	if err := CreateOrUpdateFunction(c); err != nil {
		t.Fatalf("CreateOrUpdateFunction() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, ok := function.GetFunction("plain-api")
	if !ok {
		t.Fatal("plain function was not registered")
	}
	if got.IsVariant() {
		t.Fatalf("plain function registered as variant: %#v", got)
	}
	if len(got.SupportedArchs) != 2 {
		t.Fatalf("SupportedArchs = %#v", got.SupportedArchs)
	}
}

func TestCreateVariantRequiresKnownDefaultFunction(t *testing.T) {
	resetFunctionRegistry(t)

	e := echo.New()
	payload := []byte(`{
		"Name":"missing-base-fast",
		"Runtime":"python314",
		"MemoryMB":128,
		"CPUDemand":0.1,
		"MaxConcurrency":1,
		"Handler":"fast.handler",
		"TarFunctionCode":"dGVzdA==",
		"DefaultFunction":"missing-base",
		"SpeedUp":2.0,
		"Utility":0.8
	}`)
	req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/create")

	if err := CreateOrUpdateFunction(c); err != nil {
		t.Fatalf("CreateOrUpdateFunction() error = %v", err)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateVariantAndRejectDuplicateRegistration(t *testing.T) {
	resetFunctionRegistry(t)

	base := apiTestFunction("api-base")
	if err := base.SaveToEtcd(); err != nil {
		t.Fatalf("save base: %v", err)
	}

	e := echo.New()
	payload := []byte(`{
		"Name":"api-base-fast",
		"Runtime":"python314",
		"MemoryMB":128,
		"CPUDemand":0.1,
		"MaxConcurrency":1,
		"Handler":"fast.handler",
		"TarFunctionCode":"dGVzdA==",
		"SupportedArchs":["amd64","arm64"],
		"DefaultFunction":"api-base",
		"SpeedUp":2.0,
		"Utility":0.8
	}`)

	for i, want := range []int{http.StatusOK, http.StatusConflict} {
		req := httptest.NewRequest(http.MethodPost, "/create", bytes.NewReader(payload))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/create")
		if err := CreateOrUpdateFunction(c); err != nil {
			t.Fatalf("CreateOrUpdateFunction(%d) error = %v", i, err)
		}
		if rec.Code != want {
			t.Fatalf("call %d status = %d, want %d, body = %s", i, rec.Code, want, rec.Body.String())
		}
	}

	got, ok := function.GetFunction("api-base-fast")
	if !ok {
		t.Fatal("variant was not registered")
	}
	if got.DefaultFunction != "api-base" || got.SpeedUp != 2 || got.Utility != 0.8 {
		t.Fatalf("variant metadata = %#v", got)
	}
	if len(got.SupportedArchs) != 2 || got.SupportedArchs[0] != "amd64" || got.SupportedArchs[1] != "arm64" {
		t.Fatalf("SupportedArchs = %#v", got.SupportedArchs)
	}
}

func TestGetFunctionVariantsRoute(t *testing.T) {
	resetFunctionRegistry(t)

	functions := []function.Function{
		apiTestFunction("route-base-a"),
		apiTestFunction("route-base-b"),
		apiTestVariant("route-base-a-fast", "route-base-a", 2, 0.8),
		apiTestVariant("route-base-a-small", "route-base-a", 1.5, 0.7),
		apiTestVariant("route-base-b-fast", "route-base-b", 2.2, 0.9),
	}
	for i := range functions {
		if err := functions[i].SaveToEtcd(); err != nil {
			t.Fatalf("save function %s: %v", functions[i].Name, err)
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/function/route-base-a/variants", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/function/:fun/variants")
	c.SetParamNames("fun")
	c.SetParamValues("route-base-a")

	if err := GetFunctionVariants(c); err != nil {
		t.Fatalf("GetFunctionVariants() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got []function.Function
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(variants) = %d, want 2: %#v", len(got), got)
	}
	if got[0].Name != "route-base-a-fast" || got[1].Name != "route-base-a-small" {
		t.Fatalf("variants = %#v", got)
	}
}

func TestGetFunctionVariantsUnknownFunction(t *testing.T) {
	resetFunctionRegistry(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/function/missing/variants", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/function/:fun/variants")
	c.SetParamNames("fun")
	c.SetParamValues("missing")

	if err := GetFunctionVariants(c); err != nil {
		t.Fatalf("GetFunctionVariants() error = %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}

func TestVariantFunctionAPIRejectsMalformedMetadata(t *testing.T) {
	var f function.Function
	if err := json.Unmarshal([]byte(`{
		"Name":"api-bad",
		"Runtime":"python314",
		"DefaultFunction":"api-base",
		"SpeedUp":1.0,
		"Utility":2.0
	}`), &f); err != nil {
		t.Fatalf("unmarshal API payload: %v", err)
	}
	if err := f.ValidateVariantMetadata(); err == nil {
		t.Fatal("ValidateVariantMetadata() error = nil")
	}
}

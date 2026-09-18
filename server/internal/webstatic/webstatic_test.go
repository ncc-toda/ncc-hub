package webstatic

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
)

func TestCacheControl(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want string
	}{
		{path: "/", want: htmlCache},
		{path: "/e/fes2026", want: htmlCache},
		{path: "/index.html", want: htmlCache},
		{path: "/assets/index-abc.js", want: assetCache},
		{path: "/assets/index-abc.css", want: assetCache},
	}
	for _, tc := range cases {
		if got := CacheControl(tc.path); got != tc.want {
			t.Fatalf("%s: Cache-Control = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestMountHeaders(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>ok</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(dir, "assets")
	if err := os.Mkdir(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}

	factory := func(t testing.TB) *tests.TestApp {
		app, err := tests.NewTestApp()
		if err != nil {
			t.Fatal(err)
		}
		app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
			Priority: 999,
			Func: func(se *core.ServeEvent) error {
				Mount(se, dir)
				return se.Next()
			},
		})
		return app
	}

	(&tests.ApiScenario{
		Name:            "SPA HTML は no-store で 304 にしない",
		Method:          http.MethodGet,
		URL:             "/e/fes2026",
		Headers:         map[string]string{"If-Modified-Since": "Thu, 01 Jan 1970 00:00:01 GMT"},
		ExpectedStatus:  200,
		ExpectedContent: []string{"<title>ok</title>"},
		TestAppFactory:  factory,
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			if got := res.Header.Get("Cache-Control"); got != htmlCache {
				t.Fatalf("Cache-Control = %q, want %q", got, htmlCache)
			}
		},
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "hashed asset は immutable",
		Method:          http.MethodGet,
		URL:             "/assets/app.js",
		ExpectedStatus:  200,
		ExpectedContent: []string{"console.log(1)"},
		TestAppFactory:  factory,
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			if got := res.Header.Get("Cache-Control"); got != assetCache {
				t.Fatalf("Cache-Control = %q, want %q", got, assetCache)
			}
		},
	}).Test(t)

	(&tests.ApiScenario{
		Name:               "欠けた asset は index.html に落とさない",
		Method:             http.MethodGet,
		URL:                "/assets/missing.js",
		ExpectedStatus:     404,
		NotExpectedContent: []string{"<title>ok</title>"},
		TestAppFactory:     factory,
	}).Test(t)
}
